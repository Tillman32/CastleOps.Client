package cache_test

import (
	"context"
	"fmt"
	"time"

	"github.com/castleops/client/internal/cache"
	"github.com/castleops/client/internal/config"
	"github.com/rs/zerolog"
)

// Example demonstrates basic usage of the cache package
func Example() {
	// Load configuration
	cfg := config.NewDefault()
	logger := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Logger()

	// Create cache instance (automatically selects SQLite or in-memory based on config)
	cacheInstance, err := cache.New(cfg, logger)
	if err != nil {
		panic(err)
	}
	defer cacheInstance.Close()

	ctx := context.Background()

	// Store metrics
	metrics := &cache.Metrics{
		ClientID:             "client-123",
		Timestamp:            time.Now(),
		CPUUsagePercent:      45.5,
		MemoryTotal:          16 * 1024 * 1024 * 1024,
		MemoryUsed:           8 * 1024 * 1024 * 1024,
		MemoryAvailable:      8 * 1024 * 1024 * 1024,
		MemoryUsagePercent:   50.0,
		DiskTotalBytes:       500 * 1024 * 1024 * 1024,
		DiskUsedBytes:        250 * 1024 * 1024 * 1024,
		DiskFreeBytes:        250 * 1024 * 1024 * 1024,
		DiskUsagePercent:     50.0,
		NetworkBytesReceived: 1024 * 1024 * 100,
		NetworkBytesSent:     1024 * 1024 * 50,
		Synced:               false,
	}

	if err := cacheInstance.StoreMetrics(ctx, metrics); err != nil {
		panic(err)
	}

	fmt.Printf("Stored metrics with ID: %d\n", metrics.ID)

	// Retrieve metrics
	now := time.Now()
	retrieved, err := cacheInstance.GetMetrics(ctx, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		panic(err)
	}

	fmt.Printf("Retrieved %d metrics\n", len(retrieved))

	// Store a command
	cmd := &cache.Command{
		CommandID: "cmd-456",
		Type:      "install_package",
		Payload:   `{"package": "nginx", "version": "latest"}`,
		Status:    cache.StatusPending,
		CreatedAt: time.Now(),
	}

	if err := cacheInstance.StoreCommand(ctx, cmd); err != nil {
		panic(err)
	}

	fmt.Printf("Stored command with ID: %d\n", cmd.ID)

	// Get pending commands
	pending, err := cacheInstance.GetPendingCommands(ctx)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Found %d pending commands\n", len(pending))

	// Mark command as complete
	if err := cacheInstance.MarkCommandComplete(ctx, "cmd-456"); err != nil {
		panic(err)
	}

	fmt.Println("Command marked as complete")
}

// ExampleNewSQLiteCache demonstrates creating a SQLite cache directly
func ExampleNewSQLiteCache() {
	logger := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Logger()

	// Create SQLite cache with specific configuration
	cacheInstance, err := cache.NewSQLiteCache(cache.SQLiteConfig{
		Path:          "/var/lib/castleops/cache.db",
		RetentionDays: 7,
		Logger:        logger,
	})
	if err != nil {
		panic(err)
	}
	defer cacheInstance.Close()

	fmt.Println("SQLite cache created successfully")
}

// ExampleNewMemoryCache demonstrates creating an in-memory cache directly
func ExampleNewMemoryCache() {
	logger := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Logger()

	// Create in-memory cache for testing
	cacheInstance := cache.NewMemoryCache(cache.MemoryConfig{
		RetentionDays: 7,
		Logger:        logger,
	})
	defer cacheInstance.Close()

	fmt.Println("In-memory cache created successfully")
}

// ExampleMetrics demonstrates creating and storing metrics
func ExampleMetrics() {
	logger := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Logger()
	cacheInstance := cache.NewMemoryCache(cache.MemoryConfig{
		RetentionDays: 7,
		Logger:        logger,
	})
	defer cacheInstance.Close()

	ctx := context.Background()

	// Create a metrics snapshot
	metrics := &cache.Metrics{
		ClientID:             "client-xyz",
		Timestamp:            time.Now(),
		CPUUsagePercent:      25.3,
		MemoryTotal:          32 * 1024 * 1024 * 1024, // 32GB
		MemoryUsed:           12 * 1024 * 1024 * 1024, // 12GB
		MemoryAvailable:      20 * 1024 * 1024 * 1024, // 20GB
		MemoryUsagePercent:   37.5,
		DiskTotalBytes:       1000 * 1024 * 1024 * 1024, // 1TB
		DiskUsedBytes:        600 * 1024 * 1024 * 1024,  // 600GB
		DiskFreeBytes:        400 * 1024 * 1024 * 1024,  // 400GB
		DiskUsagePercent:     60.0,
		NetworkBytesReceived: 1024 * 1024 * 200, // 200MB
		NetworkBytesSent:     1024 * 1024 * 100, // 100MB
		Synced:               false,
	}

	if err := cacheInstance.StoreMetrics(ctx, metrics); err != nil {
		panic(err)
	}

	fmt.Printf("Metrics stored with ID: %d\n", metrics.ID)
	fmt.Printf("CPU Usage: %.1f%%\n", metrics.CPUUsagePercent)
	fmt.Printf("Memory Usage: %.1f%%\n", metrics.MemoryUsagePercent)
	fmt.Printf("Disk Usage: %.1f%%\n", metrics.DiskUsagePercent)
}

// ExampleCommand demonstrates creating and storing commands
func ExampleCommand() {
	logger := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Logger()
	cacheInstance := cache.NewMemoryCache(cache.MemoryConfig{
		RetentionDays: 7,
		Logger:        logger,
	})
	defer cacheInstance.Close()

	ctx := context.Background()

	// Create an install package command
	installCmd := &cache.Command{
		CommandID: "cmd-001",
		Type:      "install_package",
		Payload:   `{"package_manager": "chocolatey", "package_name": "googlechrome", "version": "latest"}`,
		Status:    cache.StatusPending,
		CreatedAt: time.Now(),
	}

	if err := cacheInstance.StoreCommand(ctx, installCmd); err != nil {
		panic(err)
	}

	fmt.Printf("Command stored: %s\n", installCmd.Type)

	// Retrieve pending commands
	pending, err := cacheInstance.GetPendingCommands(ctx)
	if err != nil {
		panic(err)
	}

	for _, cmd := range pending {
		fmt.Printf("Pending: %s (%s)\n", cmd.CommandID, cmd.Type)

		// Process the command...
		// Then mark as complete
		if err := cacheInstance.MarkCommandComplete(ctx, cmd.CommandID); err != nil {
			panic(err)
		}
	}

	fmt.Println("All commands processed")
}
