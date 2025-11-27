package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Client is a high-performance REST API client with connection pooling,
// retry logic, and TLS support. It's designed for minimal allocations
// and maximum throughput.
type Client struct {
	// baseURL is the API server base URL (e.g., "https://api.castleops.com")
	baseURL string

	// httpClient is the underlying HTTP/2 client with connection pooling
	httpClient *http.Client

	// token is the authentication token (set after registration)
	token string

	// clientID is the unique client identifier (set after registration)
	clientID string

	// logger for structured logging
	logger zerolog.Logger

	// retryConfig controls retry behavior
	retryConfig RetryConfig

	// bufferPool reduces allocations for request/response bodies
	bufferPool *sync.Pool

	// mu protects token and clientID updates
	mu sync.RWMutex
}

// RetryConfig controls exponential backoff retry behavior
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts (default: 3)
	MaxRetries int

	// InitialBackoff is the initial backoff duration (default: 100ms)
	InitialBackoff time.Duration

	// MaxBackoff is the maximum backoff duration (default: 30s)
	MaxBackoff time.Duration

	// BackoffMultiplier is the multiplier for exponential backoff (default: 2.0)
	BackoffMultiplier float64

	// RetryableStatusCodes are HTTP status codes that trigger retries
	// Default: 408, 429, 500, 502, 503, 504
	RetryableStatusCodes map[int]bool
}

// ClientConfig contains configuration for creating a new client
type ClientConfig struct {
	// BaseURL is the API server base URL
	BaseURL string

	// TLSConfig contains TLS settings (nil for default)
	TLSConfig *tls.Config

	// Timeout is the request timeout (default: 30s)
	Timeout time.Duration

	// MaxIdleConns is the max idle connections (default: 100)
	MaxIdleConns int

	// MaxIdleConnsPerHost is max idle connections per host (default: 10)
	MaxIdleConnsPerHost int

	// IdleConnTimeout is how long idle connections are kept (default: 90s)
	IdleConnTimeout time.Duration

	// RetryConfig controls retry behavior (nil for defaults)
	RetryConfig *RetryConfig

	// Logger for structured logging
	Logger zerolog.Logger
}

// DefaultRetryConfig returns sensible retry defaults
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:        3,
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
		RetryableStatusCodes: map[int]bool{
			408: true, // Request Timeout
			429: true, // Too Many Requests
			500: true, // Internal Server Error
			502: true, // Bad Gateway
			503: true, // Service Unavailable
			504: true, // Gateway Timeout
		},
	}
}

// NewClient creates a new API client with optimized HTTP/2 transport
func NewClient(config ClientConfig) (*Client, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}

	// Set defaults
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.MaxIdleConns == 0 {
		config.MaxIdleConns = 100
	}
	if config.MaxIdleConnsPerHost == 0 {
		config.MaxIdleConnsPerHost = 10
	}
	if config.IdleConnTimeout == 0 {
		config.IdleConnTimeout = 90 * time.Second
	}

	retryConfig := DefaultRetryConfig()
	if config.RetryConfig != nil {
		retryConfig = *config.RetryConfig
	}

	// Create optimized transport with HTTP/2 support
	transport := &http.Transport{
		// Connection pooling settings
		MaxIdleConns:        config.MaxIdleConns,
		MaxIdleConnsPerHost: config.MaxIdleConnsPerHost,
		IdleConnTimeout:     config.IdleConnTimeout,

		// TCP settings for performance
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,

		// HTTP/2 settings
		ForceAttemptHTTP2: true,

		// TLS settings
		TLSClientConfig: config.TLSConfig,

		// Performance optimizations
		DisableCompression:    false, // Enable compression
		MaxConnsPerHost:       0,     // No limit
		ResponseHeaderTimeout: config.Timeout,
		ExpectContinueTimeout: 1 * time.Second,

		// Reuse connections
		DisableKeepAlives: false,
	}

	// Create HTTP client with timeout
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Limit redirects to prevent abuse
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		},
	}

	// Buffer pool for reducing allocations
	bufferPool := &sync.Pool{
		New: func() interface{} {
			// 4KB initial buffer size (typical for API requests)
			return bytes.NewBuffer(make([]byte, 0, 4096))
		},
	}

	client := &Client{
		baseURL:     config.BaseURL,
		httpClient:  httpClient,
		logger:      config.Logger,
		retryConfig: retryConfig,
		bufferPool:  bufferPool,
	}

	return client, nil
}

// NewDefaultTLSConfig creates a secure TLS configuration with modern settings
// This enforces TLS 1.3, strong cipher suites, and certificate verification
func NewDefaultTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: false,
		// Prefer modern cipher suites
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}
}

// NewMTLSConfig creates a mutual TLS configuration with client certificates
func NewMTLSConfig(certFile, keyFile, caFile string) (*tls.Config, error) {
	// Load client certificate and key
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load client cert: %w", err)
	}

	// Load CA certificate for server verification
	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA cert")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}, nil
}

// SetCredentials atomically updates client ID and auth token
func (c *Client) SetCredentials(clientID, token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clientID = clientID
	c.token = token
}

// GetCredentials atomically retrieves client ID and auth token
func (c *Client) GetCredentials() (clientID, token string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.clientID, c.token
}

// doRequest executes an HTTP request with retry logic
// This is the core request handler with exponential backoff
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	var lastErr error

	for attempt := 0; attempt <= c.retryConfig.MaxRetries; attempt++ {
		// Calculate backoff delay for retries
		if attempt > 0 {
			backoff := c.calculateBackoff(attempt)
			c.logger.Debug().
				Int("attempt", attempt).
				Dur("backoff", backoff).
				Str("path", path).
				Msg("Retrying request after backoff")

			select {
			case <-time.After(backoff):
				// Continue with retry
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Execute the request
		err := c.executeRequest(ctx, method, path, body, result)
		if err == nil {
			// Success
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !c.isRetryable(err) {
			c.logger.Debug().
				Err(err).
				Str("path", path).
				Msg("Non-retryable error, aborting")
			return err
		}

		// Log retry attempt
		c.logger.Warn().
			Err(err).
			Int("attempt", attempt+1).
			Int("max_retries", c.retryConfig.MaxRetries).
			Str("path", path).
			Msg("Request failed, will retry")
	}

	return fmt.Errorf("request failed after %d attempts: %w", c.retryConfig.MaxRetries+1, lastErr)
}

// executeRequest performs a single HTTP request
func (c *Client) executeRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	url := c.baseURL + path

	// Serialize request body
	var bodyReader io.Reader
	if body != nil {
		buf := c.getBuffer()
		defer c.putBuffer(buf)

		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return fmt.Errorf("failed to encode request: %w", err)
		}
		bodyReader = buf
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CastleOps-Client/1.0")

	// Add authentication if available
	c.mu.RLock()
	token := c.token
	c.mu.RUnlock()

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &RequestError{
			Op:  method + " " + path,
			Err: err,
		}
	}
	defer resp.Body.Close()

	// Read response body efficiently
	// Limit to 10MB to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, 10*1024*1024)
	respBody, err := io.ReadAll(limitedReader)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode >= 400 {
		// Try to parse error response
		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			return &APIError{
				StatusCode: resp.StatusCode,
				Message:    errResp.Error,
				Code:       errResp.Code,
			}
		}
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(respBody),
		}
	}

	// Decode response if result is provided
	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// calculateBackoff computes exponential backoff with jitter
func (c *Client) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff: initialBackoff * (multiplier ^ attempt)
	backoff := float64(c.retryConfig.InitialBackoff) * math.Pow(c.retryConfig.BackoffMultiplier, float64(attempt-1))

	// Cap at max backoff
	if backoff > float64(c.retryConfig.MaxBackoff) {
		backoff = float64(c.retryConfig.MaxBackoff)
	}

	// Add jitter (±25%) to prevent thundering herd
	jitter := backoff * 0.25
	backoff = backoff - jitter + (2 * jitter * float64(time.Now().UnixNano()%100) / 100)

	return time.Duration(backoff)
}

// isRetryable determines if an error should trigger a retry
func (c *Client) isRetryable(err error) bool {
	// Check for API errors with retryable status codes
	if apiErr, ok := err.(*APIError); ok {
		return c.retryConfig.RetryableStatusCodes[apiErr.StatusCode]
	}

	// Network errors are retryable
	if _, ok := err.(*RequestError); ok {
		return true
	}

	// Context errors are not retryable
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}

	// Default to not retrying unknown errors
	return false
}

// getBuffer retrieves a buffer from the pool
func (c *Client) getBuffer() *bytes.Buffer {
	buf := c.bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// putBuffer returns a buffer to the pool
func (c *Client) putBuffer(buf *bytes.Buffer) {
	// Don't pool buffers that grew too large
	if buf.Cap() > 64*1024 {
		return
	}
	c.bufferPool.Put(buf)
}

// Close releases resources held by the client
func (c *Client) Close() error {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
	return nil
}

// RequestError wraps network-level errors
type RequestError struct {
	Op  string
	Err error
}

func (e *RequestError) Error() string {
	return fmt.Sprintf("request error during %s: %v", e.Op, e.Err)
}

func (e *RequestError) Unwrap() error {
	return e.Err
}

// APIError represents an HTTP error response from the API
type APIError struct {
	StatusCode int
	Message    string
	Code       string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("API error %d (%s): %s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Message)
}

// IsRetryable returns true if this error should trigger a retry
func (e *APIError) IsRetryable() bool {
	retryable := map[int]bool{
		408: true, 429: true, 500: true, 502: true, 503: true, 504: true,
	}
	return retryable[e.StatusCode]
}

// Register performs client registration with the server
func (c *Client) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	var resp RegisterResponse
	if err := c.doRequest(ctx, http.MethodPost, "/api/v1/clients/register", req, &resp); err != nil {
		return nil, err
	}

	// Store credentials for subsequent requests
	c.SetCredentials(resp.ClientID, resp.Token)

	c.logger.Info().
		Str("client_id", resp.ClientID).
		Msg("Successfully registered with server")

	return &resp, nil
}

// Heartbeat sends a heartbeat to the server
func (c *Client) Heartbeat(ctx context.Context, req *HeartbeatRequest) (*HeartbeatResponse, error) {
	clientID, _ := c.GetCredentials()
	if clientID == "" {
		return nil, fmt.Errorf("client not registered")
	}

	path := fmt.Sprintf("/api/v1/clients/%s/heartbeat", clientID)

	var resp HeartbeatResponse
	if err := c.doRequest(ctx, http.MethodPost, path, req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// UploadMetrics uploads batched metrics to the server
func (c *Client) UploadMetrics(ctx context.Context, req *MetricsUploadRequest) (*MetricsUploadResponse, error) {
	clientID, _ := c.GetCredentials()
	if clientID == "" {
		return nil, fmt.Errorf("client not registered")
	}

	path := fmt.Sprintf("/api/v1/clients/%s/metrics", clientID)

	var resp MetricsUploadResponse
	if err := c.doRequest(ctx, http.MethodPost, path, req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// GetCommands polls for pending commands from the server
func (c *Client) GetCommands(ctx context.Context) (*CommandsResponse, error) {
	clientID, _ := c.GetCredentials()
	if clientID == "" {
		return nil, fmt.Errorf("client not registered")
	}

	path := fmt.Sprintf("/api/v1/clients/%s/commands", clientID)

	var resp CommandsResponse
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// SubmitCommandResult submits command execution results to the server
func (c *Client) SubmitCommandResult(ctx context.Context, cmdID string, req *CommandResultRequest) (*CommandResultResponse, error) {
	clientID, _ := c.GetCredentials()
	if clientID == "" {
		return nil, fmt.Errorf("client not registered")
	}

	path := fmt.Sprintf("/api/v1/clients/%s/commands/%s/result", clientID, cmdID)

	var resp CommandResultResponse
	if err := c.doRequest(ctx, http.MethodPost, path, req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}
