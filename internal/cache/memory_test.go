package cache

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// testLogger creates a no-op logger for testing
func testLogger() zerolog.Logger {
	return zerolog.New(os.Stderr).Level(zerolog.Disabled)
}

func TestNewMemoryCache(t *testing.T) {
	tests := []struct {
		name          string
		config        MemoryConfig
		wantRetention int
	}{
		{
			name: "with default retention",
			config: MemoryConfig{
				Logger: testLogger(),
			},
			wantRetention: 7,
		},
		{
			name: "with custom retention",
			config: MemoryConfig{
				RetentionDays: 30,
				Logger:        testLogger(),
			},
			wantRetention: 30,
		},
		{
			name: "with zero retention defaults to 7",
			config: MemoryConfig{
				RetentionDays: 0,
				Logger:        testLogger(),
			},
			wantRetention: 7,
		},
		{
			name: "with negative retention defaults to 7",
			config: MemoryConfig{
				RetentionDays: -5,
				Logger:        testLogger(),
			},
			wantRetention: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewMemoryCache(tt.config)
			if cache == nil {
				t.Fatal("Expected non-nil cache")
			}
			if cache.retentionDays != tt.wantRetention {
				t.Errorf("Expected retention days %d, got %d", tt.wantRetention, cache.retentionDays)
			}
			if cache.commands == nil {
				t.Error("Expected initialized commands map")
			}
			if cache.metrics == nil {
				t.Error("Expected initialized metrics slice")
			}
		})
	}
}

func TestStoreCommand(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{Logger: testLogger()})
	ctx := context.Background()

	cmd := &Command{
		CommandID: "test-cmd-1",
		Type:      "install_package",
		Payload:   `{"name": "test-package"}`,
		Status:    StatusPending,
		CreatedAt: time.Now().UTC(),
	}

	err := cache.StoreCommand(ctx, cmd)
	if err != nil {
		t.Fatalf("StoreCommand failed: %v", err)
	}

	// Verify command was stored
	if _, exists := cache.commands[cmd.CommandID]; !exists {
		t.Error("Command was not stored in cache")
	}
}

func TestStoreCommandOverwrite(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{Logger: testLogger()})
	ctx := context.Background()

	cmd1 := &Command{
		CommandID: "test-cmd-1",
		Type:      "install_package",
		Status:    StatusPending,
	}

	cmd2 := &Command{
		CommandID: "test-cmd-1",
		Type:      "uninstall_package",
		Status:    StatusComplete,
	}

	_ = cache.StoreCommand(ctx, cmd1)
	_ = cache.StoreCommand(ctx, cmd2)

	// Should have overwritten the first command
	stored := cache.commands["test-cmd-1"]
	if stored.Type != "uninstall_package" {
		t.Errorf("Expected command type 'uninstall_package', got '%s'", stored.Type)
	}
	if stored.Status != StatusComplete {
		t.Errorf("Expected status '%s', got '%s'", StatusComplete, stored.Status)
	}
}

func TestGetPendingCommands(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{Logger: testLogger()})
	ctx := context.Background()

	// Add mix of pending and completed commands
	commands := []*Command{
		{CommandID: "cmd-1", Status: StatusPending},
		{CommandID: "cmd-2", Status: StatusComplete},
		{CommandID: "cmd-3", Status: StatusPending},
		{CommandID: "cmd-4", Status: StatusFailed},
		{CommandID: "cmd-5", Status: StatusPending},
	}

	for _, cmd := range commands {
		_ = cache.StoreCommand(ctx, cmd)
	}

	pending, err := cache.GetPendingCommands(ctx)
	if err != nil {
		t.Fatalf("GetPendingCommands failed: %v", err)
	}

	if len(pending) != 3 {
		t.Errorf("Expected 3 pending commands, got %d", len(pending))
	}

	// Verify all returned commands are pending
	for _, cmd := range pending {
		if cmd.Status != StatusPending {
			t.Errorf("Expected pending status, got '%s' for command '%s'", cmd.Status, cmd.CommandID)
		}
	}
}

func TestGetPendingCommandsEmpty(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{Logger: testLogger()})
	ctx := context.Background()

	pending, err := cache.GetPendingCommands(ctx)
	if err != nil {
		t.Fatalf("GetPendingCommands failed: %v", err)
	}

	if len(pending) != 0 {
		t.Errorf("Expected 0 pending commands, got %d", len(pending))
	}
}

func TestMarkCommandComplete(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{Logger: testLogger()})
	ctx := context.Background()

	cmd := &Command{
		CommandID: "test-cmd-1",
		Status:    StatusPending,
	}

	_ = cache.StoreCommand(ctx, cmd)

	err := cache.MarkCommandComplete(ctx, "test-cmd-1")
	if err != nil {
		t.Fatalf("MarkCommandComplete failed: %v", err)
	}

	stored := cache.commands["test-cmd-1"]
	if stored.Status != StatusComplete {
		t.Errorf("Expected status '%s', got '%s'", StatusComplete, stored.Status)
	}
	if stored.CompletedAt == nil {
		t.Error("Expected CompletedAt to be set")
	}
}

func TestMarkCommandCompleteNonExistent(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{Logger: testLogger()})
	ctx := context.Background()

	// Should not error for non-existent command
	err := cache.MarkCommandComplete(ctx, "non-existent")
	if err != nil {
		t.Errorf("Expected no error for non-existent command, got: %v", err)
	}
}

func TestStoreMetrics(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{
		RetentionDays: 7,
		Logger:        testLogger(),
	})
	ctx := context.Background()

	metrics := &Metrics{
		Timestamp:       time.Now().UTC(),
		ClientID:        "test-client",
		CPUUsagePercent: 50.0,
		MemoryTotal:     16000000000,
		MemoryUsed:      8000000000,
	}

	err := cache.StoreMetrics(ctx, metrics)
	if err != nil {
		t.Fatalf("StoreMetrics failed: %v", err)
	}

	if len(cache.metrics) != 1 {
		t.Errorf("Expected 1 metric, got %d", len(cache.metrics))
	}
}

func TestStoreMetricsRetention(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{
		RetentionDays: 1,
		Logger:        testLogger(),
	})
	ctx := context.Background()

	// Add old metric (2 days ago - should be cleaned up)
	oldMetric := &Metrics{
		Timestamp: time.Now().UTC().AddDate(0, 0, -2),
		ClientID:  "test-client",
	}

	// Add recent metric (should be retained)
	recentMetric := &Metrics{
		Timestamp: time.Now().UTC(),
		ClientID:  "test-client",
	}

	_ = cache.StoreMetrics(ctx, oldMetric)
	_ = cache.StoreMetrics(ctx, recentMetric)

	// Manually trigger cleanup (since periodic cleanup won't run immediately)
	cache.Cleanup()

	// Only recent metric should remain after retention cleanup
	if len(cache.metrics) != 1 {
		t.Errorf("Expected 1 metric after retention cleanup, got %d", len(cache.metrics))
	}

	if cache.metrics[0].Timestamp != recentMetric.Timestamp {
		t.Error("Expected only recent metric to be retained")
	}
}

func TestGetMetrics(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{
		RetentionDays: 30,
		Logger:        testLogger(),
	})
	ctx := context.Background()

	now := time.Now().UTC()
	baseTime := now.Add(-24 * time.Hour) // 1 day ago

	// Add metrics at different times
	for i := 0; i < 10; i++ {
		metric := &Metrics{
			Timestamp: baseTime.Add(time.Duration(i) * time.Hour),
			ClientID:  "test-client",
		}
		_ = cache.StoreMetrics(ctx, metric)
	}

	// Query for a 5-hour window starting 2 hours after base
	start := baseTime.Add(2 * time.Hour)
	end := baseTime.Add(7 * time.Hour)

	results, err := cache.GetMetrics(ctx, start, end)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	// Should include metrics at hours 2, 3, 4, 5, 6, 7 (6 metrics)
	if len(results) != 6 {
		t.Errorf("Expected 6 metrics in range, got %d", len(results))
	}

	// Verify all results are within range
	for _, m := range results {
		if m.Timestamp.Before(start) || m.Timestamp.After(end) {
			t.Errorf("Metric timestamp %v is outside range [%v, %v]", m.Timestamp, start, end)
		}
	}
}

func TestGetMetricsEmpty(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{Logger: testLogger()})
	ctx := context.Background()

	start := time.Now().UTC().Add(-1 * time.Hour)
	end := time.Now().UTC()

	results, err := cache.GetMetrics(ctx, start, end)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 metrics, got %d", len(results))
	}
}

func TestGetMetricsNoMatch(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{
		RetentionDays: 30,
		Logger:        testLogger(),
	})
	ctx := context.Background()

	// Add metric from yesterday
	metric := &Metrics{
		Timestamp: time.Now().UTC().Add(-24 * time.Hour),
		ClientID:  "test-client",
	}
	_ = cache.StoreMetrics(ctx, metric)

	// Query for today only
	start := time.Now().UTC().Add(-1 * time.Hour)
	end := time.Now().UTC()

	results, err := cache.GetMetrics(ctx, start, end)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 metrics outside range, got %d", len(results))
	}
}

func TestClose(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{Logger: testLogger()})

	err := cache.Close()
	if err != nil {
		t.Errorf("Close should not return error, got: %v", err)
	}
}

func TestConcurrentAccess(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{
		RetentionDays: 30,
		Logger:        testLogger(),
	})
	ctx := context.Background()

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent command operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			cmd := &Command{
				CommandID: fmt.Sprintf("cmd-%d", id),
				Status:    StatusPending,
			}
			_ = cache.StoreCommand(ctx, cmd)
			_, _ = cache.GetPendingCommands(ctx)
			_ = cache.MarkCommandComplete(ctx, cmd.CommandID)
		}(i)
	}

	// Concurrent metrics operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			metric := &Metrics{
				Timestamp:       time.Now().UTC(),
				ClientID:        "test-client",
				CPUUsagePercent: float64(id),
			}
			_ = cache.StoreMetrics(ctx, metric)
			start := time.Now().UTC().Add(-1 * time.Hour)
			end := time.Now().UTC().Add(1 * time.Hour)
			_, _ = cache.GetMetrics(ctx, start, end)
		}(i)
	}

	wg.Wait()

	// If we get here without deadlock or panic, the test passes
}

func TestCacheImplementsInterface(t *testing.T) {
	// Compile-time check that MemoryCache implements Cache interface
	var _ Cache = (*MemoryCache)(nil)
}

func TestPeriodicCleanupDoesNotRunImmediately(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{
		RetentionDays: 1,
		Logger:        testLogger(),
	})
	ctx := context.Background()

	// Add old metric (2 days ago)
	oldMetric := &Metrics{
		Timestamp: time.Now().UTC().AddDate(0, 0, -2),
		ClientID:  "test-client",
	}

	_ = cache.StoreMetrics(ctx, oldMetric)

	// Old metric should still be there because cleanup interval hasn't passed
	if len(cache.metrics) != 1 {
		t.Errorf("Expected old metric to still be present, got %d metrics", len(cache.metrics))
	}
}

func TestManualCleanup(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{
		RetentionDays: 1,
		Logger:        testLogger(),
	})
	ctx := context.Background()

	// Add old metric (2 days ago)
	oldMetric := &Metrics{
		Timestamp: time.Now().UTC().AddDate(0, 0, -2),
		ClientID:  "test-client",
	}

	// Add recent metric
	recentMetric := &Metrics{
		Timestamp: time.Now().UTC(),
		ClientID:  "test-client",
	}

	_ = cache.StoreMetrics(ctx, oldMetric)
	_ = cache.StoreMetrics(ctx, recentMetric)

	// Both metrics should be present before cleanup
	if len(cache.metrics) != 2 {
		t.Errorf("Expected 2 metrics before cleanup, got %d", len(cache.metrics))
	}

	// Manually trigger cleanup
	cache.Cleanup()

	// Only recent metric should remain
	if len(cache.metrics) != 1 {
		t.Errorf("Expected 1 metric after cleanup, got %d", len(cache.metrics))
	}
}

func TestCleanupUpdatesLastCleanupTime(t *testing.T) {
	cache := NewMemoryCache(MemoryConfig{
		RetentionDays: 7,
		Logger:        testLogger(),
	})

	initialCleanup := cache.lastCleanup

	// Wait a tiny bit to ensure time difference
	time.Sleep(time.Millisecond)

	cache.Cleanup()

	if !cache.lastCleanup.After(initialCleanup) {
		t.Error("Expected lastCleanup to be updated after Cleanup()")
	}
}
