package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// testLogger creates a no-op logger for testing
func testLogger() zerolog.Logger {
	return zerolog.New(os.Stderr).Level(zerolog.Disabled)
}

// testMetrics creates a sample metrics object for testing
func testMetrics(clientID string, timestamp time.Time) *Metrics {
	return &Metrics{
		ClientID:             clientID,
		Timestamp:            timestamp,
		CPUUsagePercent:      45.5,
		MemoryTotal:          16 * 1024 * 1024 * 1024, // 16GB
		MemoryUsed:           8 * 1024 * 1024 * 1024,  // 8GB
		MemoryAvailable:      8 * 1024 * 1024 * 1024,  // 8GB
		MemoryUsagePercent:   50.0,
		DiskTotalBytes:       500 * 1024 * 1024 * 1024, // 500GB
		DiskUsedBytes:        250 * 1024 * 1024 * 1024, // 250GB
		DiskFreeBytes:        250 * 1024 * 1024 * 1024, // 250GB
		DiskUsagePercent:     50.0,
		NetworkBytesReceived: 1024 * 1024 * 100, // 100MB
		NetworkBytesSent:     1024 * 1024 * 50,  // 50MB
		Synced:               false,
	}
}

// testCommand creates a sample command for testing
func testCommand(commandID string, cmdType string) *Command {
	return &Command{
		CommandID: commandID,
		Type:      cmdType,
		Payload:   `{"package": "test-package", "version": "1.0.0"}`,
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}
}

// TestMemoryCacheBasicOperations tests basic operations on in-memory cache
func TestMemoryCacheBasicOperations(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{
		RetentionDays: 7,
		Logger:        testLogger(),
	})
	defer cache.Close()

	testCacheBasicOperations(t, cache)
}

// TestSQLiteCacheBasicOperations tests basic operations on SQLite cache
func TestSQLiteCacheBasicOperations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cache, err := NewSQLiteCache(SQLiteConfig{
		Path:          dbPath,
		RetentionDays: 7,
		Logger:        testLogger(),
	})
	if err != nil {
		t.Fatalf("failed to create SQLite cache: %v", err)
	}
	defer cache.Close()

	testCacheBasicOperations(t, cache)
}

// testCacheBasicOperations contains common tests for both cache implementations
func testCacheBasicOperations(t *testing.T, cache Cache) {
	ctx := context.Background()

	t.Run("StoreAndGetMetrics", func(t *testing.T) {
		now := time.Now().Truncate(time.Second)
		metrics := testMetrics("client-1", now)

		// Store metrics
		err := cache.StoreMetrics(ctx, metrics)
		if err != nil {
			t.Fatalf("StoreMetrics failed: %v", err)
		}

		// Verify ID was assigned
		if metrics.ID == 0 {
			t.Error("Expected non-zero ID after storage")
		}

		// Retrieve metrics
		retrieved, err := cache.GetMetrics(ctx, now.Add(-time.Minute), now.Add(time.Minute))
		if err != nil {
			t.Fatalf("GetMetrics failed: %v", err)
		}

		if len(retrieved) != 1 {
			t.Fatalf("Expected 1 metric, got %d", len(retrieved))
		}

		// Verify values
		m := retrieved[0]
		if m.ClientID != "client-1" {
			t.Errorf("Expected ClientID 'client-1', got '%s'", m.ClientID)
		}
		if m.CPUUsagePercent != 45.5 {
			t.Errorf("Expected CPUUsagePercent 45.5, got %f", m.CPUUsagePercent)
		}
	})

	t.Run("GetMetricsTimeRange", func(t *testing.T) {
		now := time.Now().Truncate(time.Second)

		// Store metrics at different times
		for i := 0; i < 5; i++ {
			m := testMetrics("client-1", now.Add(time.Duration(i)*time.Hour))
			if err := cache.StoreMetrics(ctx, m); err != nil {
				t.Fatalf("StoreMetrics failed: %v", err)
			}
		}

		// Retrieve subset (hours 1-3)
		retrieved, err := cache.GetMetrics(ctx,
			now.Add(1*time.Hour).Add(-time.Second),
			now.Add(3*time.Hour).Add(time.Second))
		if err != nil {
			t.Fatalf("GetMetrics failed: %v", err)
		}

		if len(retrieved) != 3 {
			t.Errorf("Expected 3 metrics in range, got %d", len(retrieved))
		}

		// Verify ordering (ascending by timestamp)
		for i := 1; i < len(retrieved); i++ {
			if retrieved[i].Timestamp.Before(retrieved[i-1].Timestamp) {
				t.Error("Metrics not sorted by timestamp")
			}
		}
	})

	t.Run("StoreAndGetCommand", func(t *testing.T) {
		cmd := testCommand("cmd-1", "install_package")

		// Store command
		err := cache.StoreCommand(ctx, cmd)
		if err != nil {
			t.Fatalf("StoreCommand failed: %v", err)
		}

		// Verify ID was assigned
		if cmd.ID == 0 {
			t.Error("Expected non-zero ID after storage")
		}

		// Retrieve pending commands
		pending, err := cache.GetPendingCommands(ctx)
		if err != nil {
			t.Fatalf("GetPendingCommands failed: %v", err)
		}

		if len(pending) != 1 {
			t.Fatalf("Expected 1 pending command, got %d", len(pending))
		}

		c := pending[0]
		if c.CommandID != "cmd-1" {
			t.Errorf("Expected CommandID 'cmd-1', got '%s'", c.CommandID)
		}
		if c.Type != "install_package" {
			t.Errorf("Expected Type 'install_package', got '%s'", c.Type)
		}
		if c.Status != StatusPending {
			t.Errorf("Expected Status 'pending', got '%s'", c.Status)
		}
	})

	t.Run("MarkCommandComplete", func(t *testing.T) {
		cmd := testCommand("cmd-2", "uninstall_package")

		// Store command
		err := cache.StoreCommand(ctx, cmd)
		if err != nil {
			t.Fatalf("StoreCommand failed: %v", err)
		}

		// Mark as complete
		err = cache.MarkCommandComplete(ctx, "cmd-2")
		if err != nil {
			t.Fatalf("MarkCommandComplete failed: %v", err)
		}

		// Verify it's not in pending anymore
		pending, err := cache.GetPendingCommands(ctx)
		if err != nil {
			t.Fatalf("GetPendingCommands failed: %v", err)
		}

		for _, c := range pending {
			if c.CommandID == "cmd-2" {
				t.Error("Command 'cmd-2' should not be in pending list")
			}
		}
	})

	t.Run("CommandOrdering", func(t *testing.T) {
		now := time.Now().Truncate(time.Second)

		// Store commands with different timestamps
		for i := 0; i < 3; i++ {
			cmd := testCommand(string(rune('A'+i)), "test")
			cmd.CreatedAt = now.Add(time.Duration(i) * time.Minute)
			if err := cache.StoreCommand(ctx, cmd); err != nil {
				t.Fatalf("StoreCommand failed: %v", err)
			}
		}

		// Retrieve pending commands
		pending, err := cache.GetPendingCommands(ctx)
		if err != nil {
			t.Fatalf("GetPendingCommands failed: %v", err)
		}

		// Verify ordering (ascending by created_at)
		for i := 1; i < len(pending); i++ {
			if pending[i].CreatedAt.Before(pending[i-1].CreatedAt) {
				t.Error("Commands not sorted by creation time")
			}
		}
	})
}

// TestSQLitePersistence verifies data persists across cache reopens
func TestSQLitePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "persist.db")
	ctx := context.Background()

	// Create cache and store data
	{
		cache, err := NewSQLiteCache(SQLiteConfig{
			Path:          dbPath,
			RetentionDays: 7,
			Logger:        testLogger(),
		})
		if err != nil {
			t.Fatalf("failed to create SQLite cache: %v", err)
		}

		metrics := testMetrics("client-1", time.Now())
		if err := cache.StoreMetrics(ctx, metrics); err != nil {
			t.Fatalf("StoreMetrics failed: %v", err)
		}

		cmd := testCommand("cmd-persist", "test")
		if err := cache.StoreCommand(ctx, cmd); err != nil {
			t.Fatalf("StoreCommand failed: %v", err)
		}

		cache.Close()
	}

	// Reopen cache and verify data
	{
		cache, err := NewSQLiteCache(SQLiteConfig{
			Path:          dbPath,
			RetentionDays: 7,
			Logger:        testLogger(),
		})
		if err != nil {
			t.Fatalf("failed to reopen SQLite cache: %v", err)
		}
		defer cache.Close()

		// Check metrics
		metrics, err := cache.GetMetrics(ctx, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("GetMetrics failed: %v", err)
		}
		if len(metrics) != 1 {
			t.Errorf("Expected 1 metric after reopen, got %d", len(metrics))
		}

		// Check commands
		pending, err := cache.GetPendingCommands(ctx)
		if err != nil {
			t.Fatalf("GetPendingCommands failed: %v", err)
		}
		if len(pending) != 1 {
			t.Errorf("Expected 1 pending command after reopen, got %d", len(pending))
		}
	}
}

// TestConcurrentAccess tests thread-safety of cache operations
func TestConcurrentAccess(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{
		RetentionDays: 7,
		Logger:        testLogger(),
	})
	defer cache.Close()

	ctx := context.Background()
	numGoroutines := 10
	numOperations := 100

	done := make(chan bool, numGoroutines*2)

	// Concurrent metric writes
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < numOperations; j++ {
				m := testMetrics("client-concurrent", time.Now())
				cache.StoreMetrics(ctx, m)
			}
			done <- true
		}(i)
	}

	// Concurrent metric reads
	for i := 0; i < numGoroutines; i++ {
		go func() {
			for j := 0; j < numOperations; j++ {
				cache.GetMetrics(ctx, time.Now().Add(-time.Hour), time.Now())
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines*2; i++ {
		<-done
	}
}

// BenchmarkMemoryCacheStoreMetrics benchmarks metrics storage in memory
func BenchmarkMemoryCacheStoreMetrics(b *testing.B) {
	cache := NewMemoryCache(MemoryConfig{
		RetentionDays: 7,
		Logger:        testLogger(),
	})
	defer cache.Close()

	ctx := context.Background()
	metrics := testMetrics("client-bench", time.Now())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.StoreMetrics(ctx, metrics)
	}
}

// BenchmarkSQLiteCacheStoreMetrics benchmarks metrics storage in SQLite
func BenchmarkSQLiteCacheStoreMetrics(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	cache, err := NewSQLiteCache(SQLiteConfig{
		Path:          dbPath,
		RetentionDays: 7,
		Logger:        testLogger(),
	})
	if err != nil {
		b.Fatalf("failed to create SQLite cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()
	metrics := testMetrics("client-bench", time.Now())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.StoreMetrics(ctx, metrics)
	}
}

// BenchmarkMemoryCacheGetMetrics benchmarks metrics retrieval from memory
func BenchmarkMemoryCacheGetMetrics(b *testing.B) {
	cache := NewMemoryCache(MemoryConfig{
		RetentionDays: 7,
		Logger:        testLogger(),
	})
	defer cache.Close()

	ctx := context.Background()

	// Pre-populate with 1000 metrics
	now := time.Now()
	for i := 0; i < 1000; i++ {
		m := testMetrics("client-bench", now.Add(time.Duration(i)*time.Second))
		cache.StoreMetrics(ctx, m)
	}

	start := now
	end := now.Add(1000 * time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.GetMetrics(ctx, start, end)
	}
}

// BenchmarkSQLiteCacheGetMetrics benchmarks metrics retrieval from SQLite
func BenchmarkSQLiteCacheGetMetrics(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	cache, err := NewSQLiteCache(SQLiteConfig{
		Path:          dbPath,
		RetentionDays: 7,
		Logger:        testLogger(),
	})
	if err != nil {
		b.Fatalf("failed to create SQLite cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	// Pre-populate with 1000 metrics
	now := time.Now()
	for i := 0; i < 1000; i++ {
		m := testMetrics("client-bench", now.Add(time.Duration(i)*time.Second))
		cache.StoreMetrics(ctx, m)
	}

	start := now
	end := now.Add(1000 * time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.GetMetrics(ctx, start, end)
	}
}
