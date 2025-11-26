package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// BenchmarkCPUCollector measures the performance of CPU metrics collection
func BenchmarkCPUCollector(b *testing.B) {
	buffer := newMetricsBuffer()
	collector := newCPUCollector(buffer, 5)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := collector.Collect(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMemoryCollector measures the performance of memory metrics collection
func BenchmarkMemoryCollector(b *testing.B) {
	buffer := newMetricsBuffer()
	collector := newMemoryCollector(buffer, 5)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := collector.Collect(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDiskCollector measures the performance of disk metrics collection
func BenchmarkDiskCollector(b *testing.B) {
	buffer := newMetricsBuffer()
	collector := newDiskCollector(buffer, 5, "")
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := collector.Collect(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkNetworkCollector measures the performance of network metrics collection
func BenchmarkNetworkCollector(b *testing.B) {
	buffer := newMetricsBuffer()
	collector := newNetworkCollector(buffer)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := collector.Collect(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSystemCollectOnce measures the performance of collecting all metrics once
func BenchmarkSystemCollectOnce(b *testing.B) {
	logger := zerolog.Nop()
	config := DefaultConfig()
	system, err := NewSystem(config, logger, "test-client")
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		system.collectOnce(ctx)
	}
}

// BenchmarkSystemCollectNow measures the performance of on-demand collection
func BenchmarkSystemCollectNow(b *testing.B) {
	logger := zerolog.Nop()
	config := DefaultConfig()
	system, err := NewSystem(config, logger, "test-client")
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := system.CollectNow(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMovingAverageAdd measures the performance of moving average calculations
func BenchmarkMovingAverageAdd(b *testing.B) {
	ma := newMovingAverage(10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ma.Add(float64(i % 100))
	}
}

// BenchmarkMovingAverageGet measures the performance of reading moving averages
func BenchmarkMovingAverageGet(b *testing.B) {
	ma := newMovingAverage(10)
	// Pre-populate
	for i := 0; i < 10; i++ {
		ma.Add(float64(i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ma.Get()
	}
}

// BenchmarkMetricsBufferUpdate measures the performance of updating metrics buffer
func BenchmarkMetricsBufferUpdate(b *testing.B) {
	buffer := newMetricsBuffer()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer.updateCPU(50.5)
		buffer.updateMemory(8589934592, 4294967296, 4294967296, 50.0)
		buffer.updateDisk(1099511627776, 549755813888, 549755813888, 50.0)
		buffer.updateNetwork(1000000, 500000)
	}
}

// BenchmarkMetricsPooling measures the performance of metrics object pooling
func BenchmarkMetricsPooling(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := getMetrics()
		m.CPUUsagePercent = 50.0
		m.ClientID = "test"
		putMetrics(m)
	}
}

// BenchmarkMetricsAllocation measures allocation without pooling (for comparison)
func BenchmarkMetricsAllocation(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = newMetricsBuffer()
	}
}

// BenchmarkParallelCollectors measures concurrent collection performance
func BenchmarkParallelCollectors(b *testing.B) {
	logger := zerolog.Nop()
	config := DefaultConfig()
	system, err := NewSystem(config, logger, "test-client")
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := system.CollectNow(ctx)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkGetCPUInfo measures the performance of getting static CPU information
func BenchmarkGetCPUInfo(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetCPUInfo(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetMemoryInfo measures the performance of getting detailed memory information
func BenchmarkGetMemoryInfo(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetMemoryInfo(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSystemStartStop measures the overhead of starting and stopping the system
func BenchmarkSystemStartStop(b *testing.B) {
	logger := zerolog.Nop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		config := DefaultConfig()
		config.CollectionInterval = 1 * time.Hour // Prevent actual collection during benchmark
		system, err := NewSystem(config, logger, "test-client")
		if err != nil {
			b.Fatal(err)
		}

		ctx := context.Background()
		if err := system.Start(ctx); err != nil {
			b.Fatal(err)
		}

		if err := system.Stop(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMovingAverageConcurrent measures concurrent access to moving average
func BenchmarkMovingAverageConcurrent(b *testing.B) {
	ma := newMovingAverage(10)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				ma.Add(float64(i % 100))
			} else {
				_ = ma.Get()
			}
			i++
		}
	})
}

// BenchmarkMetricsBufferConcurrent measures concurrent buffer updates
func BenchmarkMetricsBufferConcurrent(b *testing.B) {
	buffer := newMetricsBuffer()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			switch i % 4 {
			case 0:
				buffer.updateCPU(50.0)
			case 1:
				buffer.updateMemory(8589934592, 4294967296, 4294967296, 50.0)
			case 2:
				buffer.updateDisk(1099511627776, 549755813888, 549755813888, 50.0)
			case 3:
				buffer.updateNetwork(1000000, 500000)
			}
			i++
		}
	})
}
