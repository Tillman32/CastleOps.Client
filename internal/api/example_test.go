package api_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/castleops/client/internal/api"
	"github.com/castleops/client/internal/cache"
	"github.com/castleops/client/internal/config"
	"github.com/rs/zerolog"
)

// ExampleNewClient demonstrates creating a new API client
func ExampleNewClient() {
	// Create client with default settings
	client, err := api.NewClient(api.ClientConfig{
		BaseURL: "https://api.castleops.com",
		Timeout: 30 * time.Second,
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	fmt.Println("Client created successfully")
	// Output: Client created successfully
}

// ExampleNewClient_withTLS demonstrates creating a client with TLS configuration
func ExampleNewClient_withTLS() {
	// Create client with TLS 1.3
	tlsConfig := api.NewDefaultTLSConfig()

	client, err := api.NewClient(api.ClientConfig{
		BaseURL:   "https://api.castleops.com",
		TLSConfig: tlsConfig,
		Logger:    zerolog.Nop(),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	fmt.Println("Client created with TLS 1.3")
	// Output: Client created with TLS 1.3
}

// ExampleNewClient_withRetry demonstrates custom retry configuration
func ExampleNewClient_withRetry() {
	// Custom retry configuration
	retryConfig := &api.RetryConfig{
		MaxRetries:     5,
		InitialBackoff: 200 * time.Millisecond,
		MaxBackoff:     60 * time.Second,
	}

	client, err := api.NewClient(api.ClientConfig{
		BaseURL:     "https://api.castleops.com",
		RetryConfig: retryConfig,
		Logger:      zerolog.Nop(),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	fmt.Println("Client created with custom retry config")
	// Output: Client created with custom retry config
}

// ExampleClient_Register demonstrates client registration
func ExampleClient_Register() {
	client, err := api.NewClient(api.ClientConfig{
		BaseURL: "https://api.castleops.com",
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Register client
	req := &api.RegisterRequest{
		Hostname:     "my-macbook",
		OS:           "darwin",
		OSVersion:    "14.1.1",
		Architecture: "arm64",
		AgentVersion: "1.0.0",
	}

	_, err = client.Register(context.Background(), req)
	if err != nil {
		// In production, handle registration errors
		log.Printf("Registration error: %v", err)
		return
	}

	fmt.Println("Client registered successfully")
}

// ExampleClient_Heartbeat demonstrates sending heartbeats
func ExampleClient_Heartbeat() {
	client, err := api.NewClient(api.ClientConfig{
		BaseURL: "https://api.castleops.com",
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Set credentials (normally obtained from registration)
	client.SetCredentials("client-123", "token-abc")

	// Send heartbeat
	req := &api.HeartbeatRequest{
		Status:  api.ClientStatusOnline,
		Uptime:  3600,
		Version: "1.0.0",
	}

	_, err = client.Heartbeat(context.Background(), req)
	if err != nil {
		// In production, handle heartbeat errors
		log.Printf("Heartbeat error: %v", err)
		return
	}

	fmt.Println("Heartbeat sent successfully")
}

// ExampleClient_UploadMetrics demonstrates uploading metrics
func ExampleClient_UploadMetrics() {
	client, err := api.NewClient(api.ClientConfig{
		BaseURL: "https://api.castleops.com",
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	client.SetCredentials("client-123", "token-abc")

	// Prepare metrics
	metrics := []api.MetricSnapshot{
		{
			Timestamp:          time.Now().UTC(),
			CPUUsagePercent:    45.5,
			MemoryTotal:        16000000000,
			MemoryUsed:         8000000000,
			MemoryUsagePercent: 50.0,
		},
	}

	req := &api.MetricsUploadRequest{
		Metrics: metrics,
		Count:   len(metrics),
	}

	_, err = client.UploadMetrics(context.Background(), req)
	if err != nil {
		// In production, handle upload errors
		log.Printf("Upload error: %v", err)
		return
	}

	fmt.Println("Metrics uploaded successfully")
}

// ExampleRegistrationService demonstrates the registration flow
func ExampleNewRegistrationService() {
	// Create config
	cfg := config.NewDefault()

	// Create API client
	client, err := api.NewClient(api.ClientConfig{
		BaseURL: cfg.Server.URL,
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Create registration service
	regSvc := api.NewRegistrationService(api.RegistrationConfig{
		Client: client,
		Config: cfg,
		Logger: zerolog.Nop(),
	})

	// Ensure registered
	if err := regSvc.EnsureRegistered(context.Background()); err != nil {
		// In production, handle registration errors
		log.Printf("Registration error: %v", err)
		return
	}

	fmt.Println("Registration service created")
}

// ExampleCommandHandler demonstrates command handling
func ExampleNewCommandHandler() {
	// Create dependencies
	cacheInstance := cache.NewMemoryCache(cache.MemoryConfig{
		RetentionDays: 7,
		Logger:        zerolog.Nop(),
	})
	defer cacheInstance.Close()

	client, _ := api.NewClient(api.ClientConfig{
		BaseURL: "https://api.castleops.com",
		Logger:  zerolog.Nop(),
	})
	defer client.Close()

	// Create command handler
	handler := api.NewCommandHandler(api.CommandHandlerConfig{
		Client:                client,
		Cache:                 cacheInstance,
		Logger:                zerolog.Nop(),
		MaxConcurrentCommands: 5,
	})

	// Register executor for install_package command
	handler.RegisterExecutor(api.CommandInstallPackage,
		api.InstallPackageExecutor(func(ctx context.Context, payload *api.InstallPackagePayload) (string, error) {
			return fmt.Sprintf("Installed %s", payload.PackageName), nil
		}),
	)

	// Handle a command
	cmd := &api.Command{
		CommandID: "cmd-1",
		Type:      api.CommandInstallPackage,
		Payload: map[string]interface{}{
			"package_name": "googlechrome",
		},
	}

	if err := handler.HandleCommand(cmd); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Command queued for execution")
	// Output: Command queued for execution
}

// ExampleRequestQueue demonstrates offline request buffering
func ExampleNewRequestQueue() {
	// Create dependencies
	cacheInstance := cache.NewMemoryCache(cache.MemoryConfig{
		RetentionDays: 7,
		Logger:        zerolog.Nop(),
	})
	defer cacheInstance.Close()

	client, _ := api.NewClient(api.ClientConfig{
		BaseURL: "https://api.castleops.com",
		Logger:  zerolog.Nop(),
	})
	defer client.Close()

	// Create request queue
	queue := api.NewRequestQueue(api.RequestQueueConfig{
		Cache:         cacheInstance,
		Client:        client,
		Logger:        zerolog.Nop(),
		MaxQueueSize:  1000,
		RetryInterval: 30 * time.Second,
	})
	defer queue.Stop()

	// Queue metrics while offline
	queue.SetOnline(false)

	metrics := &cache.Metrics{
		Timestamp:       time.Now().UTC(),
		CPUUsagePercent: 45.5,
		MemoryTotal:     16000000000,
		MemoryUsed:      8000000000,
	}

	if err := queue.EnqueueMetrics(context.Background(), metrics); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Queue size: %d\n", queue.GetQueueSize())
	// Output: Queue size: 1
}

// ExampleClient_withContext demonstrates context usage
func ExampleClient_withContext() {
	client, err := api.NewClient(api.ClientConfig{
		BaseURL: "https://api.castleops.com",
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	client.SetCredentials("client-123", "token-abc")

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Send heartbeat with timeout
	req := &api.HeartbeatRequest{
		Status:  api.ClientStatusOnline,
		Uptime:  3600,
		Version: "1.0.0",
	}

	_, err = client.Heartbeat(ctx, req)
	if err != nil {
		// Handle timeout or other errors
		log.Printf("Heartbeat error: %v", err)
	}

	fmt.Println("Heartbeat sent with timeout")
	// Output: Heartbeat sent with timeout
}

// ExampleClient_errorHandling demonstrates error handling
func ExampleClient_errorHandling() {
	client, err := api.NewClient(api.ClientConfig{
		BaseURL: "https://api.castleops.com",
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Attempt heartbeat without credentials
	req := &api.HeartbeatRequest{
		Status:  api.ClientStatusOnline,
		Uptime:  0,
		Version: "1.0.0",
	}

	_, err = client.Heartbeat(context.Background(), req)
	if err != nil {
		// Check error type
		if apiErr, ok := err.(*api.APIError); ok {
			fmt.Printf("API error: status=%d, message=%s\n", apiErr.StatusCode, apiErr.Message)
		} else if reqErr, ok := err.(*api.RequestError); ok {
			fmt.Printf("Request error: %s\n", reqErr.Op)
		} else {
			fmt.Printf("Other error: %v\n", err)
		}
	}
}

// ExampleCommandHandler_multipleExecutors demonstrates registering multiple executors
func ExampleCommandHandler_multipleExecutors() {
	cacheInstance := cache.NewMemoryCache(cache.MemoryConfig{
		RetentionDays: 7,
		Logger:        zerolog.Nop(),
	})
	defer cacheInstance.Close()

	client, _ := api.NewClient(api.ClientConfig{
		BaseURL: "https://api.castleops.com",
		Logger:  zerolog.Nop(),
	})
	defer client.Close()

	handler := api.NewCommandHandler(api.CommandHandlerConfig{
		Client: client,
		Cache:  cacheInstance,
		Logger: zerolog.Nop(),
	})

	// Register install executor
	handler.RegisterExecutor(api.CommandInstallPackage,
		api.InstallPackageExecutor(func(ctx context.Context, p *api.InstallPackagePayload) (string, error) {
			return fmt.Sprintf("Installed %s", p.PackageName), nil
		}),
	)

	// Register uninstall executor
	handler.RegisterExecutor(api.CommandUninstallPackage,
		api.UninstallPackageExecutor(func(ctx context.Context, p *api.UninstallPackagePayload) (string, error) {
			return fmt.Sprintf("Uninstalled %s", p.PackageName), nil
		}),
	)

	// Register update executor
	handler.RegisterExecutor(api.CommandUpdatePackage,
		api.UpdatePackageExecutor(func(ctx context.Context, p *api.UpdatePackagePayload) (string, error) {
			return "Package updated", nil
		}),
	)

	fmt.Println("Registered 3 command executors")
	// Output: Registered 3 command executors
}

// ExampleRequestQueue_healthChecks demonstrates connectivity monitoring
func ExampleRequestQueue_healthChecks() {
	cacheInstance := cache.NewMemoryCache(cache.MemoryConfig{
		RetentionDays: 7,
		Logger:        zerolog.Nop(),
	})
	defer cacheInstance.Close()

	client, _ := api.NewClient(api.ClientConfig{
		BaseURL: "https://api.castleops.com",
		Logger:  zerolog.Nop(),
	})
	defer client.Close()

	queue := api.NewRequestQueue(api.RequestQueueConfig{
		Cache:  cacheInstance,
		Client: client,
		Logger: zerolog.Nop(),
	})
	defer queue.Stop()

	// Start periodic health checks
	queue.StartHealthChecks(30 * time.Second)

	fmt.Printf("Online status: %v\n", queue.IsOnline())
	// Output: Online status: true
}
