package metrics_test

import (
	"context"
	"fmt"
	"time"

	"github.com/castleops/client/internal/cache"
	"github.com/castleops/client/internal/metrics"
	"github.com/rs/zerolog"
)

// Example_basicUsage demonstrates basic metrics collection usage
func Example_basicUsage() {
	// Create a logger (use zerolog.Nop() for no output in production if desired)
	logger := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Logger()

	// Create metrics system with default configuration
	config := metrics.DefaultConfig()
	system, err := metrics.NewSystem(config, logger, "example-client-123")
	if err != nil {
		panic(err)
	}

	// Collect metrics once
	ctx := context.Background()
	metrics, err := system.CollectNow(ctx)
	if err != nil {
		panic(err)
	}

	fmt.Printf("CPU Usage: %.2f%%\n", metrics.CPUUsagePercent)
	fmt.Printf("Memory Usage: %.2f%%\n", metrics.MemoryUsagePercent)
	fmt.Printf("Disk Usage: %.2f%%\n", metrics.DiskUsagePercent)
}

// Example_periodicCollection demonstrates periodic metrics collection
func Example_periodicCollection() {
	logger := zerolog.Nop() // Silent logger for example

	// Configure collection interval
	config := metrics.DefaultConfig()
	config.CollectionInterval = 10 * time.Second

	system, err := metrics.NewSystem(config, logger, "example-client-123")
	if err != nil {
		panic(err)
	}

	// Set callback to handle collected metrics
	system.SetMetricsCallback(func(ctx context.Context, m *cache.Metrics) error {
		fmt.Printf("Metrics collected at %s\n", m.Timestamp.Format(time.RFC3339))
		fmt.Printf("  CPU: %.2f%%\n", m.CPUUsagePercent)
		fmt.Printf("  Memory: %.2f%% (%d MB used)\n",
			m.MemoryUsagePercent,
			m.MemoryUsed/1024/1024)
		return nil
	})

	// Start periodic collection
	ctx := context.Background()
	if err := system.Start(ctx); err != nil {
		panic(err)
	}

	// Run for 30 seconds
	time.Sleep(30 * time.Second)

	// Stop collection
	if err := system.Stop(); err != nil {
		panic(err)
	}
}

// Example_customConfiguration demonstrates custom configuration
func Example_customConfiguration() {
	logger := zerolog.Nop()

	// Create custom configuration
	config := &metrics.Config{
		CollectionInterval:  30 * time.Second,
		MovingAverageWindow: 10, // Use 10-sample moving average
		EnableCPU:           true,
		EnableMemory:        true,
		EnableDisk:          true,
		EnableNetwork:       false, // Disable network metrics
	}

	system, err := metrics.NewSystem(config, logger, "example-client-123")
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	metrics, err := system.CollectNow(ctx)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Collected metrics with custom config\n")
	fmt.Printf("Network bytes received: %d (should be 0, disabled)\n", metrics.NetworkBytesReceived)
}

// Example_getCPUInfo demonstrates retrieving static CPU information
func Example_getCPUInfo() {
	ctx := context.Background()
	info, err := metrics.GetCPUInfo(ctx)
	if err != nil {
		panic(err)
	}

	fmt.Printf("CPU Model: %s\n", info.ModelName)
	fmt.Printf("Physical Cores: %d\n", info.Cores)
	fmt.Printf("Logical Cores: %d\n", info.LogicalCores)
	fmt.Printf("Base Frequency: %.2f MHz\n", info.MHz)
}

// Example_getMemoryInfo demonstrates retrieving detailed memory information
func Example_getMemoryInfo() {
	ctx := context.Background()
	info, err := metrics.GetMemoryInfo(ctx)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Total Memory: %d GB\n", info.Total/1024/1024/1024)
	fmt.Printf("Used Memory: %d GB\n", info.Used/1024/1024/1024)
	fmt.Printf("Available Memory: %d GB\n", info.Available/1024/1024/1024)
	fmt.Printf("Usage: %.2f%%\n", info.UsedPercent)
}

// Example_contextCancellation demonstrates graceful cancellation
func Example_contextCancellation() {
	logger := zerolog.Nop()
	config := metrics.DefaultConfig()
	system, err := metrics.NewSystem(config, logger, "example-client-123")
	if err != nil {
		panic(err)
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start collection
	if err := system.Start(ctx); err != nil {
		panic(err)
	}

	// Wait for context cancellation
	<-ctx.Done()

	// Stop collection gracefully
	if err := system.Stop(); err != nil {
		panic(err)
	}

	fmt.Println("Collection stopped gracefully")
}

// Example_storageIntegration demonstrates integration with cache layer
func Example_storageIntegration() {
	logger := zerolog.Nop()
	config := metrics.DefaultConfig()
	config.CollectionInterval = 5 * time.Second

	system, err := metrics.NewSystem(config, logger, "example-client-123")
	if err != nil {
		panic(err)
	}

	// In real usage, you would inject your cache implementation
	// This example shows the callback pattern
	var metricsCollected int
	system.SetMetricsCallback(func(ctx context.Context, m *cache.Metrics) error {
		metricsCollected++

		// Here you would typically call:
		// return yourCache.StoreMetrics(ctx, m)

		fmt.Printf("Stored metric #%d (CPU: %.2f%%)\n", metricsCollected, m.CPUUsagePercent)
		return nil
	})

	ctx := context.Background()
	if err := system.Start(ctx); err != nil {
		panic(err)
	}

	// Collect for 15 seconds
	time.Sleep(15 * time.Second)

	if err := system.Stop(); err != nil {
		panic(err)
	}

	fmt.Printf("Total metrics collected: %d\n", metricsCollected)
}

// Example_errorHandling demonstrates error handling patterns
func Example_errorHandling() {
	logger := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Logger()

	config := metrics.DefaultConfig()
	system, err := metrics.NewSystem(config, logger, "example-client-123")
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create metrics system")
		return
	}

	// Set callback with error handling
	system.SetMetricsCallback(func(ctx context.Context, m *cache.Metrics) error {
		// Simulate storage failure
		if m.CPUUsagePercent > 90 {
			return fmt.Errorf("refusing to store metrics during high CPU usage")
		}

		fmt.Printf("Stored metrics successfully\n")
		return nil
	})

	ctx := context.Background()
	if err := system.Start(ctx); err != nil {
		logger.Error().Err(err).Msg("Failed to start metrics system")
		return
	}

	time.Sleep(2 * time.Second)

	if err := system.Stop(); err != nil {
		logger.Error().Err(err).Msg("Failed to stop metrics system")
		return
	}

	fmt.Println("System stopped successfully")
}
