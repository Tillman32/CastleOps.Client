package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// TestNewClient verifies client creation with various configurations
func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  ClientConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: ClientConfig{
				BaseURL: "https://api.example.com",
				Timeout: 30 * time.Second,
				Logger:  zerolog.Nop(),
			},
			wantErr: false,
		},
		{
			name: "minimal config with defaults",
			config: ClientConfig{
				BaseURL: "https://api.example.com",
				Logger:  zerolog.Nop(),
			},
			wantErr: false,
		},
		{
			name: "missing base URL",
			config: ClientConfig{
				Logger: zerolog.Nop(),
			},
			wantErr: true,
		},
		{
			name: "custom TLS config",
			config: ClientConfig{
				BaseURL:   "https://api.example.com",
				TLSConfig: NewDefaultTLSConfig(),
				Logger:    zerolog.Nop(),
			},
			wantErr: false,
		},
		{
			name: "custom retry config",
			config: ClientConfig{
				BaseURL: "https://api.example.com",
				RetryConfig: &RetryConfig{
					MaxRetries:     5,
					InitialBackoff: 200 * time.Millisecond,
					MaxBackoff:     60 * time.Second,
				},
				Logger: zerolog.Nop(),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Error("NewClient() returned nil client")
			}
			if client != nil {
				client.Close()
			}
		})
	}
}

// TestDefaultRetryConfig verifies default retry configuration
func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", config.MaxRetries)
	}
	if config.InitialBackoff != 100*time.Millisecond {
		t.Errorf("InitialBackoff = %v, want 100ms", config.InitialBackoff)
	}
	if config.MaxBackoff != 30*time.Second {
		t.Errorf("MaxBackoff = %v, want 30s", config.MaxBackoff)
	}
	if config.BackoffMultiplier != 2.0 {
		t.Errorf("BackoffMultiplier = %f, want 2.0", config.BackoffMultiplier)
	}

	// Verify retryable status codes
	retryableCodes := []int{408, 429, 500, 502, 503, 504}
	for _, code := range retryableCodes {
		if !config.RetryableStatusCodes[code] {
			t.Errorf("Status code %d should be retryable", code)
		}
	}
}

// TestClientCredentials verifies credential management
func TestClientCredentials(t *testing.T) {
	client, err := NewClient(ClientConfig{
		BaseURL: "https://api.example.com",
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Initially empty
	id, token := client.GetCredentials()
	if id != "" || token != "" {
		t.Errorf("Initial credentials should be empty, got id=%s, token=%s", id, token)
	}

	// Set credentials
	testID := "client-123"
	testToken := "token-abc"
	client.SetCredentials(testID, testToken)

	// Verify credentials
	id, token = client.GetCredentials()
	if id != testID {
		t.Errorf("GetCredentials() id = %s, want %s", id, testID)
	}
	if token != testToken {
		t.Errorf("GetCredentials() token = %s, want %s", token, testToken)
	}
}

// TestClientConcurrentCredentials verifies thread-safe credential access
func TestClientConcurrentCredentials(t *testing.T) {
	client, err := NewClient(ClientConfig{
		BaseURL: "https://api.example.com",
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Concurrent reads and writes
	var wg sync.WaitGroup
	iterations := 1000

	// Writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				client.SetCredentials(fmt.Sprintf("id-%d", n), fmt.Sprintf("token-%d", n))
			}
		}(i)
	}

	// Readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				client.GetCredentials()
			}
		}()
	}

	wg.Wait()
}

// TestCalculateBackoff verifies exponential backoff calculation
func TestCalculateBackoff(t *testing.T) {
	client, err := NewClient(ClientConfig{
		BaseURL: "https://api.example.com",
		RetryConfig: &RetryConfig{
			InitialBackoff:    100 * time.Millisecond,
			MaxBackoff:        10 * time.Second,
			BackoffMultiplier: 2.0,
		},
		Logger: zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	tests := []struct {
		attempt     int
		minExpected time.Duration
		maxExpected time.Duration
	}{
		{1, 75 * time.Millisecond, 125 * time.Millisecond},   // 100ms ±25%
		{2, 150 * time.Millisecond, 250 * time.Millisecond},  // 200ms ±25%
		{3, 300 * time.Millisecond, 500 * time.Millisecond},  // 400ms ±25%
		{4, 600 * time.Millisecond, 1000 * time.Millisecond}, // 800ms ±25%
		{10, 7500 * time.Millisecond, 10 * time.Second},      // Capped at maxBackoff
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			backoff := client.calculateBackoff(tt.attempt)
			if backoff < tt.minExpected || backoff > tt.maxExpected {
				t.Errorf("calculateBackoff(%d) = %v, want between %v and %v",
					tt.attempt, backoff, tt.minExpected, tt.maxExpected)
			}
		})
	}
}

// TestIsRetryable verifies retry logic for different error types
func TestIsRetryable(t *testing.T) {
	client, err := NewClient(ClientConfig{
		BaseURL: "https://api.example.com",
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	tests := []struct {
		name    string
		err     error
		want    bool
	}{
		{
			name: "retryable API error - 503",
			err:  &APIError{StatusCode: 503, Message: "Service Unavailable"},
			want: true,
		},
		{
			name: "retryable API error - 429",
			err:  &APIError{StatusCode: 429, Message: "Too Many Requests"},
			want: true,
		},
		{
			name: "non-retryable API error - 404",
			err:  &APIError{StatusCode: 404, Message: "Not Found"},
			want: false,
		},
		{
			name: "non-retryable API error - 401",
			err:  &APIError{StatusCode: 401, Message: "Unauthorized"},
			want: false,
		},
		{
			name: "request error (network)",
			err:  &RequestError{Op: "GET /api", Err: fmt.Errorf("connection refused")},
			want: true,
		},
		{
			name: "context canceled",
			err:  context.Canceled,
			want: false,
		},
		{
			name: "context deadline exceeded",
			err:  context.DeadlineExceeded,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.isRetryable(tt.err)
			if got != tt.want {
				t.Errorf("isRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestRegister verifies client registration
func TestRegister(t *testing.T) {
	// Create mock server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/clients/register" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("Unexpected method: %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Parse request
		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Verify required fields
		if req.Hostname == "" || req.OS == "" || req.Architecture == "" {
			t.Error("Missing required fields in registration request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Send response
		resp := RegisterResponse{
			ClientID:          "test-client-123",
			Token:             "test-token-abc",
			HeartbeatInterval: 30,
			MetricsInterval:   60,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// Create client
	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Test registration
	req := &RegisterRequest{
		Hostname:     "test-host",
		OS:           "darwin",
		OSVersion:    "14.0.0",
		Architecture: "arm64",
		AgentVersion: "1.0.0",
	}

	resp, err := client.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if resp.ClientID != "test-client-123" {
		t.Errorf("ClientID = %s, want test-client-123", resp.ClientID)
	}
	if resp.Token != "test-token-abc" {
		t.Errorf("Token = %s, want test-token-abc", resp.Token)
	}

	// Verify credentials were stored
	id, token := client.GetCredentials()
	if id != resp.ClientID {
		t.Errorf("Stored ClientID = %s, want %s", id, resp.ClientID)
	}
	if token != resp.Token {
		t.Errorf("Stored Token = %s, want %s", token, resp.Token)
	}
}

// TestHeartbeat verifies heartbeat functionality
func TestHeartbeat(t *testing.T) {
	// Create mock server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Authorization header = %s, want Bearer test-token", auth)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		resp := HeartbeatResponse{
			Acknowledged: true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// Create client
	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Set credentials
	client.SetCredentials("test-client", "test-token")

	// Test heartbeat
	req := &HeartbeatRequest{
		Status:  ClientStatusOnline,
		Uptime:  3600,
		Version: "1.0.0",
	}

	resp, err := client.Heartbeat(context.Background(), req)
	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}

	if !resp.Acknowledged {
		t.Error("Heartbeat not acknowledged")
	}
}

// TestRetryLogic verifies exponential backoff retry behavior
func TestRetryLogic(t *testing.T) {
	var attempts atomic.Int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := attempts.Add(1)
		if count < 3 {
			// Fail first two attempts
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// Succeed on third attempt
		resp := HeartbeatResponse{Acknowledged: true}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		RetryConfig: &RetryConfig{
			MaxRetries:     3,
			InitialBackoff: 10 * time.Millisecond,
			MaxBackoff:     100 * time.Millisecond,
		},
		Logger: zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	client.SetCredentials("test-client", "test-token")

	req := &HeartbeatRequest{
		Status:  ClientStatusOnline,
		Uptime:  0,
		Version: "1.0.0",
	}

	start := time.Now()
	resp, err := client.Heartbeat(context.Background(), req)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if !resp.Acknowledged {
		t.Error("Heartbeat not acknowledged")
	}

	// Verify it retried (should have taken at least 10ms + 20ms = 30ms)
	if elapsed < 20*time.Millisecond {
		t.Errorf("Request completed too quickly (%v), expected retries", elapsed)
	}

	finalAttempts := attempts.Load()
	if finalAttempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", finalAttempts)
	}
}

// TestContextCancellation verifies proper context handling
func TestContextCancellation(t *testing.T) {
	// Server that delays response
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		resp := HeartbeatResponse{Acknowledged: true}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Timeout: 10 * time.Second,
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	client.SetCredentials("test-client", "test-token")

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	req := &HeartbeatRequest{
		Status:  ClientStatusOnline,
		Uptime:  0,
		Version: "1.0.0",
	}

	_, err = client.Heartbeat(ctx, req)
	if err == nil {
		t.Error("Expected context deadline error, got nil")
	}
}

// TestErrorHandling verifies error response parsing
func TestErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   interface{}
		expectedError  string
		expectedCode   string
	}{
		{
			name:       "structured error response",
			statusCode: 400,
			responseBody: ErrorResponse{
				Error: "Invalid request",
				Code:  "INVALID_REQUEST",
			},
			expectedError: "Invalid request",
			expectedCode:  "INVALID_REQUEST",
		},
		{
			name:          "plain text error",
			statusCode:    500,
			responseBody:  "Internal Server Error",
			expectedError: "Internal Server Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if errResp, ok := tt.responseBody.(ErrorResponse); ok {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(errResp)
				} else {
					w.Write([]byte(tt.responseBody.(string)))
				}
			})

			server := httptest.NewServer(handler)
			defer server.Close()

			client, err := NewClient(ClientConfig{
				BaseURL: server.URL,
				Logger:  zerolog.Nop(),
			})
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer client.Close()

			client.SetCredentials("test-client", "test-token")

			req := &HeartbeatRequest{Status: ClientStatusOnline, Uptime: 0, Version: "1.0.0"}
			_, err = client.Heartbeat(context.Background(), req)

			if err == nil {
				t.Fatal("Expected error, got nil")
			}

			apiErr, ok := err.(*APIError)
			if !ok {
				t.Fatalf("Expected APIError, got %T", err)
			}

			if apiErr.StatusCode != tt.statusCode {
				t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, tt.statusCode)
			}

			if tt.expectedCode != "" && apiErr.Code != tt.expectedCode {
				t.Errorf("Code = %s, want %s", apiErr.Code, tt.expectedCode)
			}
		})
	}
}

// TestTLSConfig verifies TLS configuration
func TestTLSConfig(t *testing.T) {
	t.Run("default TLS config", func(t *testing.T) {
		config := NewDefaultTLSConfig()
		if config.MinVersion != tls.VersionTLS13 {
			t.Errorf("MinVersion = %d, want %d (TLS 1.3)", config.MinVersion, tls.VersionTLS13)
		}
		if config.InsecureSkipVerify {
			t.Error("InsecureSkipVerify should be false")
		}
	})

	t.Run("HTTPS connection", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := RegisterResponse{
				ClientID: "test-client",
				Token:    "test-token",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		// Use server's TLS config (which includes InsecureSkipVerify for testing)
		client, err := NewClient(ClientConfig{
			BaseURL:   server.URL,
			TLSConfig: server.Client().Transport.(*http.Transport).TLSClientConfig,
			Logger:    zerolog.Nop(),
		})
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

		req := &RegisterRequest{
			Hostname:     "test",
			OS:           "darwin",
			OSVersion:    "14.0",
			Architecture: "arm64",
			AgentVersion: "1.0.0",
		}

		_, err = client.Register(context.Background(), req)
		if err != nil {
			t.Errorf("Register() over HTTPS failed: %v", err)
		}
	})
}

// TestBufferPool verifies buffer pooling for reduced allocations
func TestBufferPool(t *testing.T) {
	client, err := NewClient(ClientConfig{
		BaseURL: "https://api.example.com",
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Get buffers from pool
	buf1 := client.getBuffer()
	buf2 := client.getBuffer()

	if buf1 == nil || buf2 == nil {
		t.Fatal("getBuffer() returned nil")
	}

	// Buffers should be different instances
	if buf1 == buf2 {
		t.Error("getBuffer() returned same buffer instance")
	}

	// Write some data
	buf1.WriteString("test data")
	if buf1.Len() != 9 {
		t.Errorf("Buffer length = %d, want 9", buf1.Len())
	}

	// Return to pool
	client.putBuffer(buf1)

	// Get again - should be reset
	buf3 := client.getBuffer()
	if buf3.Len() != 0 {
		t.Errorf("Recycled buffer length = %d, want 0", buf3.Len())
	}

	client.putBuffer(buf2)
	client.putBuffer(buf3)
}

// TestUploadMetrics verifies metrics upload
func TestUploadMetrics(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req MetricsUploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if req.Count != len(req.Metrics) {
			t.Errorf("Count mismatch: %d != %d", req.Count, len(req.Metrics))
		}

		resp := MetricsUploadResponse{
			Received:     req.Count,
			Acknowledged: true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	client.SetCredentials("test-client", "test-token")

	req := &MetricsUploadRequest{
		Metrics: []MetricSnapshot{
			{
				Timestamp:       time.Now().UTC(),
				CPUUsagePercent: 45.5,
				MemoryTotal:     16000000000,
				MemoryUsed:      8000000000,
			},
			{
				Timestamp:       time.Now().UTC().Add(-time.Minute),
				CPUUsagePercent: 42.3,
				MemoryTotal:     16000000000,
				MemoryUsed:      7500000000,
			},
		},
		Count: 2,
	}

	resp, err := client.UploadMetrics(context.Background(), req)
	if err != nil {
		t.Fatalf("UploadMetrics() error = %v", err)
	}

	if resp.Received != 2 {
		t.Errorf("Received = %d, want 2", resp.Received)
	}
	if !resp.Acknowledged {
		t.Error("Metrics not acknowledged")
	}
}

// TestGetCommands verifies command polling
func TestGetCommands(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := CommandsResponse{
			Commands: []*Command{
				{
					CommandID: "cmd-1",
					Type:      CommandInstallPackage,
					Payload: map[string]interface{}{
						"package_name": "test-package",
					},
				},
			},
			Count: 1,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	client.SetCredentials("test-client", "test-token")

	resp, err := client.GetCommands(context.Background())
	if err != nil {
		t.Fatalf("GetCommands() error = %v", err)
	}

	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
	if len(resp.Commands) != 1 {
		t.Errorf("len(Commands) = %d, want 1", len(resp.Commands))
	}
	if resp.Commands[0].CommandID != "cmd-1" {
		t.Errorf("CommandID = %s, want cmd-1", resp.Commands[0].CommandID)
	}
}

// TestSubmitCommandResult verifies command result submission
func TestSubmitCommandResult(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req CommandResultRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if req.CommandID != "cmd-1" {
			t.Errorf("CommandID = %s, want cmd-1", req.CommandID)
		}
		if req.Status != CommandStatusSuccess {
			t.Errorf("Status = %s, want %s", req.Status, CommandStatusSuccess)
		}

		resp := CommandResultResponse{Acknowledged: true}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	client.SetCredentials("test-client", "test-token")

	req := &CommandResultRequest{
		CommandID:     "cmd-1",
		Status:        CommandStatusSuccess,
		Output:        "Package installed successfully",
		ExecutionTime: 1500,
		CompletedAt:   time.Now().UTC(),
	}

	resp, err := client.SubmitCommandResult(context.Background(), "cmd-1", req)
	if err != nil {
		t.Fatalf("SubmitCommandResult() error = %v", err)
	}

	if !resp.Acknowledged {
		t.Error("Result not acknowledged")
	}
}
