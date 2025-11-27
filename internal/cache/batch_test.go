package cache

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// TestBatchStoreMetricsMemory tests batch metrics storage in memory cache
func TestBatchStoreMetricsMemory(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{
		RetentionDays: 7,
		Logger:        testLogger(),
	})
	defer cache.Close()

	testBatchStoreMetrics(t, cache)
}

// TestBatchStoreMetricsSQLite tests batch metrics storage in SQLite cache
func TestBatchStoreMetricsSQLite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "batch.db")

	cache, err := NewSQLiteCache(SQLiteConfig{
		Path:          dbPath,
		RetentionDays: 7,
		Logger:        testLogger(),
	})
	if err != nil {
		t.Fatalf("failed to create SQLite cache: %v", err)
	}
	defer cache.Close()

	testBatchStoreMetrics(t, cache)
}

// testBatchStoreMetrics contains common batch store tests
func testBatchStoreMetrics(t *testing.T, cache Cache) {
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	t.Run("BatchStoreEmpty", func(t *testing.T) {
		count, err := BatchStoreMetrics(ctx, cache, []*Metrics{})
		if err != nil {
			t.Errorf("BatchStoreMetrics failed: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected count 0, got %d", count)
		}
	})

	t.Run("BatchStoreMultiple", func(t *testing.T) {
		// Create 100 metrics
		metrics := make([]*Metrics, 100)
		for i := 0; i < 100; i++ {
			metrics[i] = testMetrics("batch-client", now.Add(time.Duration(i)*time.Second))
		}

		count, err := BatchStoreMetrics(ctx, cache, metrics)
		if err != nil {
			t.Fatalf("BatchStoreMetrics failed: %v", err)
		}
		if count != 100 {
			t.Errorf("Expected count 100, got %d", count)
		}

		// Verify all metrics were stored
		retrieved, err := cache.GetMetrics(ctx, now.Add(-time.Second), now.Add(100*time.Second))
		if err != nil {
			t.Fatalf("GetMetrics failed: %v", err)
		}
		if len(retrieved) < 100 {
			t.Errorf("Expected at least 100 metrics, got %d", len(retrieved))
		}
	})

	t.Run("BatchStoreWithNils", func(t *testing.T) {
		// Mix valid metrics with nils
		metrics := []*Metrics{
			testMetrics("client-1", now),
			nil,
			testMetrics("client-2", now),
			nil,
			testMetrics("client-3", now),
		}

		count, err := BatchStoreMetrics(ctx, cache, metrics)
		if err != nil {
			t.Fatalf("BatchStoreMetrics failed: %v", err)
		}
		if count != 3 {
			t.Errorf("Expected count 3, got %d", count)
		}
	})
}

// TestBatchStoreCommandsMemory tests batch command storage in memory cache
func TestBatchStoreCommandsMemory(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{
		RetentionDays: 7,
		Logger:        testLogger(),
	})
	defer cache.Close()

	testBatchStoreCommands(t, cache)
}

// TestBatchStoreCommandsSQLite tests batch command storage in SQLite cache
func TestBatchStoreCommandsSQLite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "batch.db")

	cache, err := NewSQLiteCache(SQLiteConfig{
		Path:          dbPath,
		RetentionDays: 7,
		Logger:        testLogger(),
	})
	if err != nil {
		t.Fatalf("failed to create SQLite cache: %v", err)
	}
	defer cache.Close()

	testBatchStoreCommands(t, cache)
}

// testBatchStoreCommands contains common batch command store tests
func testBatchStoreCommands(t *testing.T, cache Cache) {
	ctx := context.Background()

	t.Run("BatchStoreEmpty", func(t *testing.T) {
		count, err := BatchStoreCommands(ctx, cache, []*Command{})
		if err != nil {
			t.Errorf("BatchStoreCommands failed: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected count 0, got %d", count)
		}
	})

	t.Run("BatchStoreMultiple", func(t *testing.T) {
		// Create 50 commands
		commands := make([]*Command, 50)
		for i := 0; i < 50; i++ {
			commands[i] = testCommand(string(rune('A'+i)), "install_package")
		}

		count, err := BatchStoreCommands(ctx, cache, commands)
		if err != nil {
			t.Fatalf("BatchStoreCommands failed: %v", err)
		}
		if count != 50 {
			t.Errorf("Expected count 50, got %d", count)
		}

		// Verify all commands were stored
		pending, err := cache.GetPendingCommands(ctx)
		if err != nil {
			t.Fatalf("GetPendingCommands failed: %v", err)
		}
		if len(pending) < 50 {
			t.Errorf("Expected at least 50 commands, got %d", len(pending))
		}
	})

	t.Run("BatchStoreWithNils", func(t *testing.T) {
		// Mix valid commands with nils
		commands := []*Command{
			testCommand("cmd-nil-1", "test"),
			nil,
			testCommand("cmd-nil-2", "test"),
			nil,
			testCommand("cmd-nil-3", "test"),
		}

		count, err := BatchStoreCommands(ctx, cache, commands)
		if err != nil {
			t.Fatalf("BatchStoreCommands failed: %v", err)
		}
		if count != 3 {
			t.Errorf("Expected count 3, got %d", count)
		}
	})
}

// BenchmarkBatchStoreMetricsSQLite benchmarks batch metrics storage
func BenchmarkBatchStoreMetricsSQLite(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench-batch.db")

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
	now := time.Now()

	// Prepare batch of 100 metrics
	metrics := make([]*Metrics, 100)
	for i := 0; i < 100; i++ {
		metrics[i] = testMetrics("bench-client", now.Add(time.Duration(i)*time.Second))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BatchStoreMetrics(ctx, cache, metrics)
	}
}

// BenchmarkIndividualStoreMetricsSQLite benchmarks individual metrics storage for comparison
func BenchmarkIndividualStoreMetricsSQLite(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench-individual.db")

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
	now := time.Now()

	// Prepare batch of 100 metrics
	metrics := make([]*Metrics, 100)
	for i := 0; i < 100; i++ {
		metrics[i] = testMetrics("bench-client", now.Add(time.Duration(i)*time.Second))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, m := range metrics {
			cache.StoreMetrics(ctx, m)
		}
	}
}
