package agent

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/castleops/client/internal/api"
	"github.com/castleops/client/internal/config"
	"github.com/rs/zerolog"
)

const (
	// AgentVersion is the current version of the client software
	AgentVersion = "0.1.0"

	// maxRegistrationRetries is the maximum number of registration attempts
	maxRegistrationRetries = 5

	// registrationRetryDelay is the base delay between registration retries
	registrationRetryDelay = 5 * time.Second
)

// RegistrationService handles client registration with the server
// It manages the registration flow, credential storage, and re-registration
// when credentials become invalid.
type RegistrationService struct {
	apiClient *api.Client
	config    *config.Config
	logger    zerolog.Logger
}

// NewRegistrationService creates a new registration service
func NewRegistrationService(apiClient *api.Client, config *config.Config, logger zerolog.Logger) *RegistrationService {
	return &RegistrationService{
		apiClient: apiClient,
		config:    config,
		logger:    logger.With().Str("component", "registration").Logger(),
	}
}

// EnsureRegistered ensures the client is registered with the server
// If already registered with valid credentials, returns immediately
// Otherwise, performs registration and stores credentials
// Returns (clientID, token, error)
func (r *RegistrationService) EnsureRegistered(ctx context.Context) (string, string, error) {
	// Check if we already have credentials
	clientID, token := r.config.GetClientCredentials()
	if clientID != "" && token != "" {
		// Verify credentials are still valid
		if r.verifyCredentials(ctx) {
			r.logger.Info().Str("client_id", clientID).Msg("Using existing valid credentials")
			return clientID, token, nil
		}

		// Credentials are invalid, need to re-register
		r.logger.Warn().Msg("Existing credentials are invalid, re-registering")
	}

	// Perform registration
	return r.Register(ctx)
}

// Register performs client registration with the server
// This collects system information and sends it to the server
// Returns (clientID, token, error)
func (r *RegistrationService) Register(ctx context.Context) (string, string, error) {
	r.logger.Info().Msg("Starting client registration")

	// Collect system information for registration
	req, err := r.buildRegistrationRequest()
	if err != nil {
		return "", "", fmt.Errorf("failed to build registration request: %w", err)
	}

	// Attempt registration with exponential backoff
	var lastErr error
	for attempt := 1; attempt <= maxRegistrationRetries; attempt++ {
		// Add timeout for each attempt
		attemptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)

		resp, err := r.apiClient.Register(attemptCtx, req)
		cancel()

		if err == nil {
			// Registration successful
			r.logger.Info().
				Str("client_id", resp.ClientID).
				Int("attempt", attempt).
				Msg("Registration successful")

			// Store credentials in config
			r.config.UpdateClientCredentials(resp.ClientID, resp.Token)

			// Update intervals if server provided them
			if resp.HeartbeatInterval > 0 {
				r.logger.Info().
					Int("heartbeat_interval", resp.HeartbeatInterval).
					Msg("Server provided heartbeat interval")
				// Note: Config updates would need to be implemented to persist these
			}

			if resp.MetricsInterval > 0 {
				r.logger.Info().
					Int("metrics_interval", resp.MetricsInterval).
					Msg("Server provided metrics interval")
			}

			return resp.ClientID, resp.Token, nil
		}

		lastErr = err

		// Log the failure
		r.logger.Warn().
			Err(err).
			Int("attempt", attempt).
			Int("max_retries", maxRegistrationRetries).
			Msg("Registration attempt failed")

		// Check if we should retry
		if attempt < maxRegistrationRetries {
			// Calculate backoff with exponential increase
			backoff := time.Duration(attempt) * registrationRetryDelay

			r.logger.Info().
				Dur("backoff", backoff).
				Int("next_attempt", attempt+1).
				Msg("Retrying registration after backoff")

			select {
			case <-time.After(backoff):
				// Continue to next attempt
			case <-ctx.Done():
				return "", "", ctx.Err()
			}
		}
	}

	return "", "", fmt.Errorf("registration failed after %d attempts: %w", maxRegistrationRetries, lastErr)
}

// buildRegistrationRequest collects system information for registration
func (r *RegistrationService) buildRegistrationRequest() (*api.RegisterRequest, error) {
	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		r.logger.Warn().Err(err).Msg("Failed to get hostname, using default")
		hostname = "unknown"
	}

	// Get OS information
	osName := runtime.GOOS
	osVersion := r.getOSVersion()

	// Get architecture
	architecture := runtime.GOARCH

	req := &api.RegisterRequest{
		Hostname:     hostname,
		OS:           osName,
		OSVersion:    osVersion,
		Architecture: architecture,
		AgentVersion: AgentVersion,
	}

	r.logger.Debug().
		Str("hostname", hostname).
		Str("os", osName).
		Str("os_version", osVersion).
		Str("architecture", architecture).
		Str("agent_version", AgentVersion).
		Msg("Built registration request")

	return req, nil
}

// getOSVersion attempts to retrieve the OS version string
func (r *RegistrationService) getOSVersion() string {
	// This is platform-specific and would need to be implemented
	// for each target OS. For now, return a basic version.
	switch runtime.GOOS {
	case "darwin":
		// Could use `sw_vers -productVersion` to get macOS version
		return "macOS"
	case "windows":
		// Could use Windows API to get version
		return "Windows"
	case "linux":
		// Could parse /etc/os-release
		return "Linux"
	default:
		return "unknown"
	}
}

// verifyCredentials checks if the current credentials are valid
// This sends a lightweight request to verify the token works
func (r *RegistrationService) verifyCredentials(ctx context.Context) bool {
	// Create a context with short timeout for verification
	verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Try to send a heartbeat as a credential check
	// If the heartbeat succeeds, credentials are valid
	req := &api.HeartbeatRequest{
		Status:  api.ClientStatusOnline,
		Uptime:  0,
		Version: AgentVersion,
	}

	_, err := r.apiClient.Heartbeat(verifyCtx, req)
	if err != nil {
		r.logger.Debug().Err(err).Msg("Credential verification failed")
		return false
	}

	return true
}

// ReRegister forces a re-registration even if credentials exist
// This is useful when credentials are known to be invalid
func (r *RegistrationService) ReRegister(ctx context.Context) (string, string, error) {
	r.logger.Info().Msg("Forcing re-registration")

	// Clear existing credentials
	r.config.UpdateClientCredentials("", "")

	// Perform registration
	return r.Register(ctx)
}

// GetClientID returns the current client ID if registered
func (r *RegistrationService) GetClientID() string {
	clientID, _ := r.config.GetClientCredentials()
	return clientID
}

// GetToken returns the current auth token if registered
func (r *RegistrationService) GetToken() string {
	_, token := r.config.GetClientCredentials()
	return token
}

// IsRegistered returns true if we have valid credentials
func (r *RegistrationService) IsRegistered() bool {
	clientID, token := r.config.GetClientCredentials()
	return clientID != "" && token != ""
}
