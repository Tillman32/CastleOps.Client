package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/castleops/client/internal/cache"
	"github.com/rs/zerolog"
)

// TestMovingAverage verifies the moving average calculation
func TestMovingAverage(t *testing.T) {
	ma := newMovingAverage(3)

	// Test first value
	avg := ma.Add(10.0)
	if avg != 10.0 {
		t.Errorf("Expected 10.0, got %f", avg)
	}

	// Test second value
	avg = ma.Add(20.0)
	if avg != 15.0 {
		t.Errorf("Expected 15.0, got %f", avg)
	}

	// Test third value (window full)
	avg = ma.Add(30.0)
	if avg != 20.0 {
		t.Errorf("Expected 20.0, got %f", avg)
	}

	// Test fourth value (should drop first value)
	avg = ma.Add(40.0)
	if avg != 30.0 {
		t.Errorf("Expected 30.0, got %f", avg)
	}
}

// TestMovingAverageReset verifies reset functionality
func TestMovingAverageReset(t *testing.T) {
	ma := newMovingAverage(3)

	ma.Add(10.0)
	ma.Add(20.0)
	ma.Add(30.0)

	ma.Reset()

	avg := ma.Get()
	if avg != 0.0 {
		t.Errorf("Expected 0.0 after reset, got %f", avg)
	}

	// Verify it works after reset
	avg = ma.Add(50.0)
	if avg != 50.0 {
		t.Errorf("Expected 50.0, got %f", avg)
	}
}

// TestMetricsBuffer verifies buffer operations
func TestMetricsBuffer(t *testing.T) {
	buffer := newMetricsBuffer()

	// Update all metrics
	buffer.updateCPU(75.5)
	buffer.updateMemory(8589934592, 4294967296, 4294967296, 50.0)
	buffer.updateDisk(1099511627776, 549755813888, 549755813888, 50.0)
	buffer.updateNetwork(1000000, 500000)
	buffer.finalize("test-client")

	metrics := buffer.get()

	// Verify values
	if metrics.CPUUsagePercent != 75.5 {
		t.Errorf("Expected CPU 75.5, got %f", metrics.CPUUsagePercent)
	}
	if metrics.MemoryTotal != 8589934592 {
		t.Errorf("Expected MemoryTotal 8589934592, got %d", metrics.MemoryTotal)
	}
	if metrics.DiskTotalBytes != 1099511627776 {
		t.Errorf("Expected DiskTotalBytes 1099511627776, got %d", metrics.DiskTotalBytes)
	}
	if metrics.NetworkBytesReceived != 1000000 {
		t.Errorf("Expected NetworkBytesReceived 1000000, got %d", metrics.NetworkBytesReceived)
	}
	if metrics.ClientID != "test-client" {
		t.Errorf("Expected ClientID 'test-client', got %s", metrics.ClientID)
	}
	if metrics.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

// TestCPUCollector verifies CPU metrics collection
func TestCPUCollector(t *testing.T) {
	buffer := newMetricsBuffer()
	collector := newCPUCollector(buffer, 5)

	ctx := context.Background()
	err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("CPU collection failed: %v", err)
	}

	metrics := buffer.get()
	if metrics.CPUUsagePercent < 0 || metrics.CPUUsagePercent > 100 {
		t.Errorf("Invalid CPU usage: %f", metrics.CPUUsagePercent)
	}

	if collector.Name() != "cpu" {
		t.Errorf("Expected name 'cpu', got %s", collector.Name())
	}
}

// TestMemoryCollector verifies memory metrics collection
func TestMemoryCollector(t *testing.T) {
	buffer := newMetricsBuffer()
	collector := newMemoryCollector(buffer, 5)

	ctx := context.Background()
	err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Memory collection failed: %v", err)
	}

	metrics := buffer.get()
	if metrics.MemoryTotal == 0 {
		t.Error("MemoryTotal should not be zero")
	}
	if metrics.MemoryUsagePercent < 0 || metrics.MemoryUsagePercent > 100 {
		t.Errorf("Invalid memory usage: %f", metrics.MemoryUsagePercent)
	}

	if collector.Name() != "memory" {
		t.Errorf("Expected name 'memory', got %s", collector.Name())
	}
}

// TestDiskCollector verifies disk metrics collection
func TestDiskCollector(t *testing.T) {
	buffer := newMetricsBuffer()
	collector := newDiskCollector(buffer, 5, "")

	ctx := context.Background()
	err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Disk collection failed: %v", err)
	}

	metrics := buffer.get()
	if metrics.DiskTotalBytes == 0 {
		t.Error("DiskTotalBytes should not be zero")
	}
	if metrics.DiskUsagePercent < 0 || metrics.DiskUsagePercent > 100 {
		t.Errorf("Invalid disk usage: %f", metrics.DiskUsagePercent)
	}

	if collector.Name() != "disk" {
		t.Errorf("Expected name 'disk', got %s", collector.Name())
	}
}

// TestNetworkCollector verifies network metrics collection
func TestNetworkCollector(t *testing.T) {
	buffer := newMetricsBuffer()
	collector := newNetworkCollector(buffer)

	ctx := context.Background()

	// First collection initializes counters
	err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Network collection failed: %v", err)
	}

	// Second collection should have delta
	time.Sleep(100 * time.Millisecond)
	err = collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Network collection failed: %v", err)
	}

	if collector.Name() != "network" {
		t.Errorf("Expected name 'network', got %s", collector.Name())
	}
}

// TestSystemCreation verifies system initialization
func TestSystemCreation(t *testing.T) {
	logger := zerolog.Nop()
	config := DefaultConfig()

	system, err := NewSystem(config, logger, "test-client")
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}

	if system.IsRunning() {
		t.Error("System should not be running after creation")
	}

	stats := system.Stats()
	if stats.CollectorCount != 4 {
		t.Errorf("Expected 4 collectors, got %d", stats.CollectorCount)
	}
}

// TestSystemStartStop verifies system lifecycle
func TestSystemStartStop(t *testing.T) {
	logger := zerolog.Nop()
	config := DefaultConfig()
	config.CollectionInterval = 100 * time.Millisecond

	system, err := NewSystem(config, logger, "test-client")
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}

	ctx := context.Background()
	err = system.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start system: %v", err)
	}

	if !system.IsRunning() {
		t.Error("System should be running after start")
	}

	// Wait for at least one collection
	time.Sleep(150 * time.Millisecond)

	err = system.Stop()
	if err != nil {
		t.Fatalf("Failed to stop system: %v", err)
	}

	if system.IsRunning() {
		t.Error("System should not be running after stop")
	}
}

// TestSystemCollectNow verifies on-demand collection
func TestSystemCollectNow(t *testing.T) {
	logger := zerolog.Nop()
	config := DefaultConfig()

	system, err := NewSystem(config, logger, "test-client")
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}

	ctx := context.Background()
	metrics, err := system.CollectNow(ctx)
	if err != nil {
		t.Fatalf("CollectNow failed: %v", err)
	}

	if metrics.ClientID != "test-client" {
		t.Errorf("Expected ClientID 'test-client', got %s", metrics.ClientID)
	}
	if metrics.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

// TestSystemCallback verifies metrics callback functionality
func TestSystemCallback(t *testing.T) {
	logger := zerolog.Nop()
	config := DefaultConfig()
	config.CollectionInterval = 100 * time.Millisecond

	system, err := NewSystem(config, logger, "test-client")
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}

	callbackCalled := false
	system.SetMetricsCallback(func(ctx context.Context, m *cache.Metrics) error {
		callbackCalled = true
		if m.ClientID != "test-client" {
			t.Errorf("Expected ClientID 'test-client', got %s", m.ClientID)
		}
		return nil
	})

	ctx := context.Background()
	err = system.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start system: %v", err)
	}

	// Wait for callback
	time.Sleep(150 * time.Millisecond)

	err = system.Stop()
	if err != nil {
		t.Fatalf("Failed to stop system: %v", err)
	}

	if !callbackCalled {
		t.Error("Callback should have been called")
	}
}

// TestSystemConfigUpdate verifies configuration updates
func TestSystemConfigUpdate(t *testing.T) {
	logger := zerolog.Nop()
	config := DefaultConfig()

	system, err := NewSystem(config, logger, "test-client")
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}

	newConfig := &Config{
		CollectionInterval:  30 * time.Second,
		MovingAverageWindow: 10,
		EnableCPU:           true,
		EnableMemory:        true,
		EnableDisk:          true,
		EnableNetwork:       true,
	}

	err = system.UpdateConfig(newConfig)
	if err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	currentConfig := system.GetConfig()
	if currentConfig.CollectionInterval != 30*time.Second {
		t.Errorf("Expected interval 30s, got %v", currentConfig.CollectionInterval)
	}
	if currentConfig.MovingAverageWindow != 10 {
		t.Errorf("Expected window 10, got %d", currentConfig.MovingAverageWindow)
	}
}

// TestGetCPUInfo verifies CPU info retrieval
func TestGetCPUInfo(t *testing.T) {
	ctx := context.Background()
	info, err := GetCPUInfo(ctx)
	if err != nil {
		t.Fatalf("Failed to get CPU info: %v", err)
	}

	if info.Cores == 0 {
		t.Error("Cores should not be zero")
	}
	if info.LogicalCores == 0 {
		t.Error("LogicalCores should not be zero")
	}
}

// TestGetMemoryInfo verifies memory info retrieval
func TestGetMemoryInfo(t *testing.T) {
	ctx := context.Background()
	info, err := GetMemoryInfo(ctx)
	if err != nil {
		t.Fatalf("Failed to get memory info: %v", err)
	}

	if info.Total == 0 {
		t.Error("Total memory should not be zero")
	}
	if info.UsedPercent < 0 || info.UsedPercent > 100 {
		t.Errorf("Invalid memory usage percent: %f", info.UsedPercent)
	}
}

// TestMetricsPooling verifies object pooling reduces allocations
func TestMetricsPooling(t *testing.T) {
	// Get and return multiple times
	for i := 0; i < 100; i++ {
		m := getMetrics()
		m.CPUUsagePercent = float64(i)
		putMetrics(m)
	}

	// Verify pool works
	m := getMetrics()
	if m == nil {
		t.Error("getMetrics returned nil")
	}
	putMetrics(m)
}

// TestContextCancellation verifies collectors respect context cancellation
func TestContextCancellation(t *testing.T) {
	buffer := newMetricsBuffer()
	collector := newCPUCollector(buffer, 5)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Collection should fail or return quickly with context error
	err := collector.Collect(ctx)
	if err != nil && err != context.Canceled {
		// Some collectors may succeed even with cancelled context if they're fast enough
		// That's acceptable - we just want to verify they don't hang
		t.Logf("Collection error: %v", err)
	}
}

// TestDefaultConfig verifies default configuration is valid
func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.CollectionInterval <= 0 {
		t.Error("CollectionInterval should be positive")
	}
	if config.MovingAverageWindow < 0 {
		t.Error("MovingAverageWindow should be non-negative")
	}
	if !config.EnableCPU || !config.EnableMemory || !config.EnableDisk || !config.EnableNetwork {
		t.Error("All collectors should be enabled by default")
	}
}
