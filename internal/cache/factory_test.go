package cache

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/castleops/client/internal/config"
	"github.com/rs/zerolog"
)

func TestNew(t *testing.T) {
	logger := zerolog.Nop()

	tests := []struct {
		name        string
		cacheType   string
		wantErr     bool
		errContains string
	}{
		{
			name:      "memory cache type",
			cacheType: "memory",
			wantErr:   false,
		},
		{
			name:      "empty cache type defaults to memory",
			cacheType: "",
			wantErr:   false,
		},
		{
			name:      "sqlite falls back to memory with warning",
			cacheType: "sqlite",
			wantErr:   false,
		},
		{
			name:        "unsupported cache type returns error",
			cacheType:   "redis",
			wantErr:     true,
			errContains: "unsupported cache type: redis",
		},
		{
			name:        "another unsupported type",
			cacheType:   "postgresql",
			wantErr:     true,
			errContains: "unsupported cache type: postgresql",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Cache: config.CacheConfig{
					Type: tt.cacheType,
				},
				Metrics: config.MetricsConfig{
					RetentionDays: 7,
				},
			}

			cache, err := New(cfg, logger)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errContains, err.Error())
				}
				if cache != nil {
					t.Error("Expected nil cache when error occurs")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}
				if cache == nil {
					t.Error("Expected non-nil cache")
					return
				}

				// Verify it's a working cache by using it
				memCache, ok := cache.(*MemoryCache)
				if !ok {
					t.Error("Expected MemoryCache type")
					return
				}
				if memCache == nil {
					t.Error("Expected non-nil MemoryCache")
				}
			}
		})
	}
}

func TestNewPassesRetentionDays(t *testing.T) {
	logger := zerolog.Nop()

	cfg := &config.Config{
		Cache: config.CacheConfig{
			Type: "memory",
		},
		Metrics: config.MetricsConfig{
			RetentionDays: 30,
		},
	}

	cache, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	memCache, ok := cache.(*MemoryCache)
	if !ok {
		t.Fatal("Expected MemoryCache type")
	}

	if memCache.retentionDays != 30 {
		t.Errorf("Expected retention days 30, got %d", memCache.retentionDays)
	}
}

func TestNewCacheIsFunctional(t *testing.T) {
	logger := zerolog.Nop()

	cfg := &config.Config{
		Cache: config.CacheConfig{
			Type: "memory",
		},
		Metrics: config.MetricsConfig{
			RetentionDays: 7,
		},
	}

	cache, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Test basic functionality
	ctx := context.Background()

	// Test command operations
	cmd := &Command{
		CommandID: "test-cmd",
		Status:    StatusPending,
		CreatedAt: time.Now().UTC(),
	}
	if err := cache.StoreCommand(ctx, cmd); err != nil {
		t.Errorf("StoreCommand failed: %v", err)
	}

	pending, err := cache.GetPendingCommands(ctx)
	if err != nil {
		t.Errorf("GetPendingCommands failed: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("Expected 1 pending command, got %d", len(pending))
	}

	if err := cache.MarkCommandComplete(ctx, "test-cmd"); err != nil {
		t.Errorf("MarkCommandComplete failed: %v", err)
	}

	// Test metrics operations
	metrics := &Metrics{
		Timestamp: time.Now().UTC(),
		ClientID:  "test-client",
	}
	if err := cache.StoreMetrics(ctx, metrics); err != nil {
		t.Errorf("StoreMetrics failed: %v", err)
	}

	start := time.Now().UTC().Add(-1 * time.Hour)
	end := time.Now().UTC().Add(1 * time.Hour)
	results, err := cache.GetMetrics(ctx, start, end)
	if err != nil {
		t.Errorf("GetMetrics failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 metric, got %d", len(results))
	}

	// Test close
	if err := cache.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}
