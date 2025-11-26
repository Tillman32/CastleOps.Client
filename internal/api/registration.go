package api

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/castleops/client/internal/config"
	"github.com/rs/zerolog"
	"github.com/shirou/gopsutil/v4/host"
)

// RegistrationService manages client registration with the API server
type RegistrationService struct {
	client *Client
	config *config.Config
	logger zerolog.Logger
}

// RegistrationConfig configures the registration service
type RegistrationConfig struct {
	Client *Client
	Config *config.Config
	Logger zerolog.Logger
}

// NewRegistrationService creates a new registration service
func NewRegistrationService(cfg RegistrationConfig) *RegistrationService {
	return &RegistrationService{
		client: cfg.Client,
		config: cfg.Config,
		logger: cfg.Logger,
	}
}

// Register performs initial client registration with the server
// It collects system information and obtains credentials
func (r *RegistrationService) Register(ctx context.Context) (*RegisterResponse, error) {
	// Check if already registered
	clientID, _ := r.config.GetClientCredentials()
	if clientID != "" {
		r.logger.Warn().
			Str("client_id", clientID).
			Msg("Client already registered, skipping registration")
		return nil, fmt.Errorf("client already registered with ID: %s", clientID)
	}

	r.logger.Info().Msg("Starting client registration")

	// Gather system information
	req, err := r.buildRegistrationRequest()
	if err != nil {
		return nil, fmt.Errorf("failed to build registration request: %w", err)
	}

	// Log registration details (excluding sensitive info)
	r.logger.Info().
		Str("hostname", req.Hostname).
		Str("os", req.OS).
		Str("os_version", req.OSVersion).
		Str("architecture", req.Architecture).
		Str("agent_version", req.AgentVersion).
		Msg("Registering client with server")

	// Perform registration with retry
	resp, err := r.registerWithRetry(ctx, req, 3)
	if err != nil {
		return nil, fmt.Errorf("registration failed: %w", err)
	}

	// Store credentials in config
	r.config.UpdateClientCredentials(resp.ClientID, resp.Token)

	// Log successful registration
	r.logger.Info().
		Str("client_id", resp.ClientID).
		Int("heartbeat_interval", resp.HeartbeatInterval).
		Int("metrics_interval", resp.MetricsInterval).
		Msg("Client registration successful")

	return resp, nil
}

// registerWithRetry attempts registration with exponential backoff
func (r *RegistrationService) registerWithRetry(ctx context.Context, req *RegisterRequest, maxRetries int) (*RegisterResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			r.logger.Info().
				Int("attempt", attempt).
				Dur("backoff", backoff).
				Msg("Retrying registration after backoff")

			select {
			case <-time.After(backoff):
				// Continue with retry
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		resp, err := r.client.Register(ctx, req)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		r.logger.Warn().
			Err(err).
			Int("attempt", attempt+1).
			Int("max_retries", maxRetries).
			Msg("Registration attempt failed")
	}

	return nil, fmt.Errorf("registration failed after %d attempts: %w", maxRetries+1, lastErr)
}

// buildRegistrationRequest gathers system information for registration
func (r *RegistrationService) buildRegistrationRequest() (*RegisterRequest, error) {
	req := &RegisterRequest{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		AgentVersion: r.getAgentVersion(),
	}

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		r.logger.Warn().Err(err).Msg("Failed to get hostname, using default")
		hostname = "unknown"
	}
	req.Hostname = hostname

	// Get OS version using gopsutil
	osVersion, err := r.getOSVersion()
	if err != nil {
		r.logger.Warn().Err(err).Msg("Failed to get OS version, using default")
		osVersion = "unknown"
	}
	req.OSVersion = osVersion

	return req, nil
}

// getOSVersion retrieves the operating system version
func (r *RegistrationService) getOSVersion() (string, error) {
	info, err := host.Info()
	if err != nil {
		return "", err
	}

	// Format: Platform PlatformVersion (e.g., "darwin 14.1.1" or "windows 10.0.19045")
	return fmt.Sprintf("%s %s", info.Platform, info.PlatformVersion), nil
}

// getAgentVersion returns the agent version
// In production, this would come from build flags
func (r *RegistrationService) getAgentVersion() string {
	// This would typically be set during build:
	// go build -ldflags "-X main.version=1.0.0"
	version := "1.0.0-dev"
	return version
}

// IsRegistered checks if the client is already registered
func (r *RegistrationService) IsRegistered() bool {
	clientID, token := r.config.GetClientCredentials()
	return clientID != "" && token != ""
}

// GetClientID returns the current client ID
func (r *RegistrationService) GetClientID() string {
	clientID, _ := r.config.GetClientCredentials()
	return clientID
}

// Reregister forces a new registration (useful after credentials expire)
// This is typically only needed if the server invalidated the client
func (r *RegistrationService) Reregister(ctx context.Context) (*RegisterResponse, error) {
	r.logger.Warn().Msg("Forcing client re-registration")

	// Clear existing credentials
	r.config.UpdateClientCredentials("", "")
	r.client.SetCredentials("", "")

	// Perform fresh registration
	return r.Register(ctx)
}

// ValidateCredentials checks if stored credentials are valid
// by attempting a heartbeat request
func (r *RegistrationService) ValidateCredentials(ctx context.Context) error {
	if !r.IsRegistered() {
		return fmt.Errorf("client not registered")
	}

	// Attempt a heartbeat to validate credentials
	req := &HeartbeatRequest{
		Status:  ClientStatusOnline,
		Uptime:  0,
		Version: r.getAgentVersion(),
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := r.client.Heartbeat(timeoutCtx, req)
	if err != nil {
		// Check if it's an auth error
		if apiErr, ok := err.(*APIError); ok {
			if apiErr.StatusCode == 401 || apiErr.StatusCode == 403 {
				return fmt.Errorf("credentials invalid or expired")
			}
		}
		return fmt.Errorf("failed to validate credentials: %w", err)
	}

	r.logger.Debug().Msg("Credentials validated successfully")
	return nil
}

// EnsureRegistered ensures the client is registered, registering if necessary
// This is a convenience method for initialization
func (r *RegistrationService) EnsureRegistered(ctx context.Context) error {
	if r.IsRegistered() {
		// Validate existing credentials
		if err := r.ValidateCredentials(ctx); err != nil {
			r.logger.Warn().
				Err(err).
				Msg("Existing credentials invalid, re-registering")

			// Credentials are invalid, re-register
			if _, err := r.Reregister(ctx); err != nil {
				return fmt.Errorf("re-registration failed: %w", err)
			}
			return nil
		}

		r.logger.Debug().Msg("Client already registered with valid credentials")
		return nil
	}

	// Not registered, perform initial registration
	if _, err := r.Register(ctx); err != nil {
		return fmt.Errorf("initial registration failed: %w", err)
	}

	return nil
}
