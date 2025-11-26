package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// BenchmarkClientCreation measures client initialization overhead
func BenchmarkClientCreation(b *testing.B) {
	config := ClientConfig{
		BaseURL: "https://api.example.com",
		Logger:  zerolog.Nop(),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		client, err := NewClient(config)
		if err != nil {
			b.Fatal(err)
		}
		client.Close()
	}
}

// BenchmarkBufferPooling measures buffer pool performance
func BenchmarkBufferPooling(b *testing.B) {
	client, err := NewClient(ClientConfig{
		BaseURL: "https://api.example.com",
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		b.Fatal(err)
	}
	defer client.Close()

	b.Run("GetBuffer", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buf := client.getBuffer()
			client.putBuffer(buf)
		}
	})

	b.Run("GetBuffer_Parallel", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				buf := client.getBuffer()
				client.putBuffer(buf)
			}
		})
	})
}

// BenchmarkCredentialAccess measures concurrent credential access performance
func BenchmarkCredentialAccess(b *testing.B) {
	client, err := NewClient(ClientConfig{
		BaseURL: "https://api.example.com",
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		b.Fatal(err)
	}
	defer client.Close()

	client.SetCredentials("test-client-123", "test-token-abc")

	b.Run("GetCredentials", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			client.GetCredentials()
		}
	})

	b.Run("GetCredentials_Parallel", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				client.GetCredentials()
			}
		})
	})

	b.Run("SetCredentials", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			client.SetCredentials("test-client-123", "test-token-abc")
		}
	})
}

// BenchmarkRegister measures registration request performance
func BenchmarkRegister(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		b.Fatal(err)
	}
	defer client.Close()

	req := &RegisterRequest{
		Hostname:     "test-host",
		OS:           "darwin",
		OSVersion:    "14.0.0",
		Architecture: "arm64",
		AgentVersion: "1.0.0",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.Register(context.Background(), req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHeartbeat measures heartbeat request performance
func BenchmarkHeartbeat(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := HeartbeatResponse{Acknowledged: true}
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
		b.Fatal(err)
	}
	defer client.Close()

	client.SetCredentials("test-client", "test-token")

	req := &HeartbeatRequest{
		Status:  ClientStatusOnline,
		Uptime:  3600,
		Version: "1.0.0",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.Heartbeat(context.Background(), req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHeartbeat_Concurrent measures concurrent heartbeat performance
func BenchmarkHeartbeat_Concurrent(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := HeartbeatResponse{Acknowledged: true}
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
		b.Fatal(err)
	}
	defer client.Close()

	client.SetCredentials("test-client", "test-token")

	req := &HeartbeatRequest{
		Status:  ClientStatusOnline,
		Uptime:  3600,
		Version: "1.0.0",
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := client.Heartbeat(context.Background(), req)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkUploadMetrics measures metrics upload performance
func BenchmarkUploadMetrics(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req MetricsUploadRequest
		json.NewDecoder(r.Body).Decode(&req)

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
		b.Fatal(err)
	}
	defer client.Close()

	client.SetCredentials("test-client", "test-token")

	// Test with different batch sizes
	batchSizes := []int{1, 10, 50, 100}

	for _, size := range batchSizes {
		b.Run(string(rune('0'+size)), func(b *testing.B) {
			metrics := make([]MetricSnapshot, size)
			now := time.Now().UTC()

			for i := 0; i < size; i++ {
				metrics[i] = MetricSnapshot{
					Timestamp:            now.Add(-time.Duration(i) * time.Minute),
					CPUUsagePercent:      45.5,
					MemoryTotal:          16000000000,
					MemoryUsed:           8000000000,
					MemoryAvailable:      8000000000,
					MemoryUsagePercent:   50.0,
					DiskTotalBytes:       500000000000,
					DiskUsedBytes:        250000000000,
					DiskFreeBytes:        250000000000,
					DiskUsagePercent:     50.0,
					NetworkBytesReceived: 1000000,
					NetworkBytesSent:     500000,
				}
			}

			req := &MetricsUploadRequest{
				Metrics: metrics,
				Count:   size,
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := client.UploadMetrics(context.Background(), req)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkGetCommands measures command polling performance
func BenchmarkGetCommands(b *testing.B) {
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
		b.Fatal(err)
	}
	defer client.Close()

	client.SetCredentials("test-client", "test-token")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.GetCommands(context.Background())
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSubmitCommandResult measures command result submission performance
func BenchmarkSubmitCommandResult(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		b.Fatal(err)
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

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.SubmitCommandResult(context.Background(), "cmd-1", req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCalculateBackoff measures backoff calculation performance
func BenchmarkCalculateBackoff(b *testing.B) {
	client, err := NewClient(ClientConfig{
		BaseURL: "https://api.example.com",
		Logger:  zerolog.Nop(),
	})
	if err != nil {
		b.Fatal(err)
	}
	defer client.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = client.calculateBackoff(3)
	}
}

// BenchmarkRetryLogic measures retry overhead
func BenchmarkRetryLogic(b *testing.B) {
	attempts := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts%3 != 0 {
			// Fail 2 out of 3 requests
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
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
			InitialBackoff: 1 * time.Millisecond, // Very short for benchmarking
			MaxBackoff:     10 * time.Millisecond,
		},
		Logger: zerolog.Nop(),
	})
	if err != nil {
		b.Fatal(err)
	}
	defer client.Close()

	client.SetCredentials("test-client", "test-token")

	req := &HeartbeatRequest{
		Status:  ClientStatusOnline,
		Uptime:  0,
		Version: "1.0.0",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.Heartbeat(context.Background(), req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkJSONEncoding measures JSON serialization performance
func BenchmarkJSONEncoding(b *testing.B) {
	req := &RegisterRequest{
		Hostname:     "test-host",
		OS:           "darwin",
		OSVersion:    "14.0.0",
		Architecture: "arm64",
		AgentVersion: "1.0.0",
	}

	b.Run("Marshal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := json.Marshal(req)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Unmarshal", func(b *testing.B) {
		data, _ := json.Marshal(req)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var decoded RegisterRequest
			err := json.Unmarshal(data, &decoded)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkConnectionReuse measures HTTP connection pooling efficiency
func BenchmarkConnectionReuse(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := HeartbeatResponse{Acknowledged: true}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	b.Run("WithPooling", func(b *testing.B) {
		client, err := NewClient(ClientConfig{
			BaseURL:             server.URL,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			Logger:              zerolog.Nop(),
		})
		if err != nil {
			b.Fatal(err)
		}
		defer client.Close()

		client.SetCredentials("test-client", "test-token")

		req := &HeartbeatRequest{
			Status:  ClientStatusOnline,
			Uptime:  0,
			Version: "1.0.0",
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_, err := client.Heartbeat(context.Background(), req)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("WithoutPooling", func(b *testing.B) {
		client, err := NewClient(ClientConfig{
			BaseURL:             server.URL,
			MaxIdleConns:        0,
			MaxIdleConnsPerHost: 0,
			Logger:              zerolog.Nop(),
		})
		if err != nil {
			b.Fatal(err)
		}
		defer client.Close()

		client.SetCredentials("test-client", "test-token")

		req := &HeartbeatRequest{
			Status:  ClientStatusOnline,
			Uptime:  0,
			Version: "1.0.0",
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_, err := client.Heartbeat(context.Background(), req)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
