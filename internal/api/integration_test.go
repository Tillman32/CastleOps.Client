package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/castleops/client/internal/cache"
	"github.com/castleops/client/internal/config"
	"github.com/rs/zerolog"
)

// TestIntegration_FullRegistrationFlow tests the complete registration workflow
func TestIntegration_FullRegistrationFlow(t *testing.T) {
	// Create mock server
	var registeredClients sync.Map

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/clients/register":
			var req RegisterRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("Failed to decode registration: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			clientID := "client-" + req.Hostname
			token := "token-" + req.Hostname

			registeredClients.Store(clientID, token)

			resp := RegisterResponse{
				ClientID:          clientID,
				Token:             token,
				HeartbeatInterval: 30,
				MetricsInterval:   60,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		default:
			// Verify authentication for all other endpoints
			auth := r.Header.Get("Authorization")
			if auth == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// Extract client ID from path
			clientID := r.URL.Path
			if storedToken, ok := registeredClients.Load(clientID); ok {
				expectedAuth := "Bearer " + storedToken.(string)
				if auth != expectedAuth {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
			}

			// Handle heartbeat
			if r.Method == http.MethodPost && r.URL.Path != "/api/v1/clients/register" {
				resp := HeartbeatResponse{Acknowledged: true}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			}
		}
	}))
	defer server.Close()

	// Create config
	cfg := config.NewDefault()
	cfg.Server.URL = server.URL

	// Create client
	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create registration service
	regSvc := NewRegistrationService(RegistrationConfig{
		Client: client,
		Config: cfg,
		Logger: zerolog.Nop(),
	})

	// Test registration
	resp, err := regSvc.Register(context.Background())
	if err != nil {
		t.Fatalf("Registration failed: %v", err)
	}

	if resp.ClientID == "" {
		t.Error("ClientID should not be empty")
	}
	if resp.Token == "" {
		t.Error("Token should not be empty")
	}

	// Verify credentials were stored
	id, token := cfg.GetClientCredentials()
	if id != resp.ClientID {
		t.Errorf("Config ClientID = %s, want %s", id, resp.ClientID)
	}
	if token != resp.Token {
		t.Errorf("Config Token = %s, want %s", token, resp.Token)
	}

	// Test that duplicate registration fails
	_, err = regSvc.Register(context.Background())
	if err == nil {
		t.Error("Expected error for duplicate registration")
	}
}

// TestIntegration_HeartbeatLoop tests periodic heartbeat sending
func TestIntegration_HeartbeatLoop(t *testing.T) {
	var heartbeatCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		heartbeatCount.Add(1)
		resp := HeartbeatResponse{Acknowledged: true}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
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

	// Simulate heartbeat loop
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			req := &HeartbeatRequest{
				Status:  ClientStatusOnline,
				Uptime:  0,
				Version: "1.0.0",
			}
			if _, err := client.Heartbeat(context.Background(), req); err != nil {
				t.Errorf("Heartbeat failed: %v", err)
			}
		case <-ctx.Done():
			goto Done
		}
	}

Done:
	count := heartbeatCount.Load()
	if count < 3 {
		t.Errorf("Expected at least 3 heartbeats, got %d", count)
	}
}

// TestIntegration_MetricsUploadBatch tests batched metrics upload
func TestIntegration_MetricsUploadBatch(t *testing.T) {
	var receivedMetrics []MetricSnapshot
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req MetricsUploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode metrics: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		mu.Lock()
		receivedMetrics = append(receivedMetrics, req.Metrics...)
		mu.Unlock()

		resp := MetricsUploadResponse{
			Received:     req.Count,
			Acknowledged: true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
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

	// Upload multiple batches
	batches := 3
	batchSize := 10

	for i := 0; i < batches; i++ {
		metrics := make([]MetricSnapshot, batchSize)
		now := time.Now().UTC()

		for j := 0; j < batchSize; j++ {
			metrics[j] = MetricSnapshot{
				Timestamp:       now.Add(-time.Duration(j) * time.Minute),
				CPUUsagePercent: float64(40 + j),
				MemoryTotal:     16000000000,
				MemoryUsed:      8000000000,
			}
		}

		req := &MetricsUploadRequest{
			Metrics: metrics,
			Count:   batchSize,
		}

		if _, err := client.UploadMetrics(context.Background(), req); err != nil {
			t.Fatalf("Batch %d failed: %v", i, err)
		}
	}

	mu.Lock()
	totalReceived := len(receivedMetrics)
	mu.Unlock()

	expected := batches * batchSize
	if totalReceived != expected {
		t.Errorf("Received %d metrics, want %d", totalReceived, expected)
	}
}

// TestIntegration_CommandExecution tests end-to-end command execution
func TestIntegration_CommandExecution(t *testing.T) {
	var executedCommands sync.Map
	var submittedResults sync.Map

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path != "/api/v1/clients/register":
			// Get commands
			resp := CommandsResponse{
				Commands: []*Command{
					{
						CommandID: "cmd-1",
						Type:      CommandInstallPackage,
						Payload: map[string]interface{}{
							"package_manager": "homebrew",
							"package_name":    "wget",
						},
					},
				},
				Count: 1,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case r.Method == http.MethodPost && r.URL.Path != "/api/v1/clients/register":
			// Submit command result
			var req CommandResultRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			submittedResults.Store(req.CommandID, req)

			resp := CommandResultResponse{Acknowledged: true}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	// Create cache
	cacheInstance := cache.NewMemoryCache(cache.MemoryConfig{
		RetentionDays: 7,
		Logger:        zerolog.Nop(),
	})
	defer cacheInstance.Close()

	// Create client
	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	client.SetCredentials("test-client", "test-token")

	// Create command handler
	handler := NewCommandHandler(CommandHandlerConfig{
		Client:                client,
		Cache:                 cacheInstance,
		Logger:                zerolog.Nop(),
		MaxConcurrentCommands: 5,
	})

	// Register executor
	handler.RegisterExecutor(CommandInstallPackage,
		InstallPackageExecutor(func(ctx context.Context, payload *InstallPackagePayload) (string, error) {
			executedCommands.Store(payload.PackageName, true)
			return "Package installed", nil
		}),
	)

	// Get commands
	commands, err := client.GetCommands(context.Background())
	if err != nil {
		t.Fatalf("GetCommands failed: %v", err)
	}

	if commands.Count != 1 {
		t.Fatalf("Expected 1 command, got %d", commands.Count)
	}

	// Handle command
	if err := handler.HandleCommand(commands.Commands[0]); err != nil {
		t.Fatalf("HandleCommand failed: %v", err)
	}

	// Wait for execution
	time.Sleep(100 * time.Millisecond)

	// Verify command was executed
	if _, ok := executedCommands.Load("wget"); !ok {
		t.Error("Command was not executed")
	}

	// Stop handler to flush results
	if err := handler.Stop(5 * time.Second); err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	// Verify result was submitted
	if _, ok := submittedResults.Load("cmd-1"); !ok {
		t.Error("Command result was not submitted")
	}
}

// TestIntegration_OfflineQueueing tests offline request buffering
func TestIntegration_OfflineQueueing(t *testing.T) {
	serverAvailable := atomic.Bool{}
	serverAvailable.Store(true)

	var receivedMetrics atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !serverAvailable.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		var req MetricsUploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		receivedMetrics.Add(int32(req.Count))

		resp := MetricsUploadResponse{
			Received:     req.Count,
			Acknowledged: true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create cache
	cacheInstance := cache.NewMemoryCache(cache.MemoryConfig{
		RetentionDays: 7,
		Logger:        zerolog.Nop(),
	})
	defer cacheInstance.Close()

	// Create client
	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	client.SetCredentials("test-client", "test-token")

	// Create request queue
	queue := NewRequestQueue(RequestQueueConfig{
		Cache:         cacheInstance,
		Client:        client,
		Logger:        zerolog.Nop(),
		RetryInterval: 100 * time.Millisecond,
	})
	defer queue.Stop()

	// Simulate going offline
	serverAvailable.Store(false)
	queue.SetOnline(false)

	// Queue metrics while offline
	for i := 0; i < 5; i++ {
		metrics := &cache.Metrics{
			Timestamp:       time.Now().UTC(),
			CPUUsagePercent: float64(40 + i),
			MemoryTotal:     16000000000,
			MemoryUsed:      8000000000,
		}
		if err := queue.EnqueueMetrics(context.Background(), metrics); err != nil {
			t.Errorf("Failed to enqueue metrics: %v", err)
		}
	}

	queueSize := queue.GetQueueSize()
	if queueSize != 5 {
		t.Errorf("Queue size = %d, want 5", queueSize)
	}

	// Come back online
	serverAvailable.Store(true)
	queue.SetOnline(true)

	// Wait for queue to flush
	time.Sleep(500 * time.Millisecond)

	received := receivedMetrics.Load()
	if received != 5 {
		t.Errorf("Received %d metrics, want 5", received)
	}
}

// TestIntegration_ConcurrentRequests tests concurrent API usage
func TestIntegration_ConcurrentRequests(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		resp := HeartbeatResponse{Acknowledged: true}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
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

	// Send concurrent heartbeats
	concurrency := 50
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req := &HeartbeatRequest{
				Status:  ClientStatusOnline,
				Uptime:  0,
				Version: "1.0.0",
			}

			if _, err := client.Heartbeat(context.Background(), req); err != nil {
				t.Errorf("Heartbeat failed: %v", err)
			}
		}()
	}

	wg.Wait()

	count := requestCount.Load()
	if count != int32(concurrency) {
		t.Errorf("Received %d requests, want %d", count, concurrency)
	}
}

// TestIntegration_RetryRecovery tests automatic retry on transient failures
func TestIntegration_RetryRecovery(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := attempts.Add(1)

		// Fail first 2 attempts
		if count < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		resp := HeartbeatResponse{Acknowledged: true}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
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

	resp, err := client.Heartbeat(context.Background(), req)
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	if !resp.Acknowledged {
		t.Error("Heartbeat not acknowledged")
	}

	finalAttempts := attempts.Load()
	if finalAttempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", finalAttempts)
	}
}
