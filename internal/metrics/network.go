package metrics

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v4/net"
)

// networkCollector gathers network usage metrics
// Tracks cumulative bytes sent/received and calculates rates over time
type networkCollector struct {
	buffer            *metricsBuffer
	lastBytesReceived uint64
	lastBytesSent     uint64
	initialized       bool
	mu                sync.Mutex
}

// newNetworkCollector creates a new network metrics collector
// Network metrics are cumulative counters that must be diffed over time
func newNetworkCollector(buffer *metricsBuffer) *networkCollector {
	return &networkCollector{
		buffer: buffer,
	}
}

// Name returns the collector name for logging and debugging
func (c *networkCollector) Name() string {
	return "network"
}

// Collect gathers network usage metrics
// Network counters are cumulative since boot, so we track deltas between collections
// On the first call, we initialize counters without reporting deltas
func (c *networkCollector) Collect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create a channel for the result to allow context cancellation
	type result struct {
		counters []net.IOCountersStat
		err      error
	}
	resultChan := make(chan result, 1)

	// Perform network collection in a goroutine to allow context cancellation
	go func() {
		// pernic=false aggregates all interfaces into a single stat
		// This gives us total system network activity
		counters, err := net.IOCountersWithContext(ctx, false)
		resultChan <- result{counters: counters, err: err}
	}()

	// Wait for result or context cancellation
	select {
	case res := <-resultChan:
		if res.err != nil {
			return fmt.Errorf("network collection failed: %w", res.err)
		}

		if len(res.counters) == 0 {
			return fmt.Errorf("no network counter data returned")
		}

		counter := res.counters[0] // Aggregated stats for all interfaces

		// On first collection, just initialize counters
		if !c.initialized {
			c.lastBytesReceived = counter.BytesRecv
			c.lastBytesSent = counter.BytesSent
			c.initialized = true

			// Set initial values to 0 (no delta yet)
			c.buffer.updateNetwork(0, 0)
			return nil
		}

		// Update metrics buffer with cumulative totals
		// Note: We store cumulative values, not deltas
		// Consumers can calculate deltas between samples if needed
		c.buffer.updateNetwork(counter.BytesRecv, counter.BytesSent)

		// Update last known values for next delta calculation
		c.lastBytesReceived = counter.BytesRecv
		c.lastBytesSent = counter.BytesSent

		return nil

	case <-ctx.Done():
		// Context cancelled, return error
		return ctx.Err()
	}
}

// Reset clears the counter history
// Call this when you want to start fresh measurements
func (c *networkCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initialized = false
	c.lastBytesReceived = 0
	c.lastBytesSent = 0
}

// GetNetworkInterfaces returns detailed information about all network interfaces
// This is useful for discovering available interfaces and their configurations
func GetNetworkInterfaces(ctx context.Context) ([]InterfaceInfo, error) {
	// Get interface stats
	interfaces, err := net.InterfacesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	// Get I/O counters per interface
	ioCounters, err := net.IOCountersWithContext(ctx, true) // pernic=true
	if err != nil {
		// If we can't get I/O stats, continue with interface info only
		ioCounters = nil
	}

	// Build I/O counter map for quick lookup
	ioMap := make(map[string]net.IOCountersStat)
	if ioCounters != nil {
		for _, counter := range ioCounters {
			ioMap[counter.Name] = counter
		}
	}

	result := make([]InterfaceInfo, 0, len(interfaces))
	for _, iface := range interfaces {
		info := InterfaceInfo{
			Index:        iface.Index,
			MTU:          iface.MTU,
			Name:         iface.Name,
			HardwareAddr: iface.HardwareAddr,
			Flags:        iface.Flags,
		}

		// Add IP addresses
		for _, addr := range iface.Addrs {
			info.Addresses = append(info.Addresses, addr.Addr)
		}

		// Add I/O stats if available
		if counter, ok := ioMap[iface.Name]; ok {
			info.BytesReceived = counter.BytesRecv
			info.BytesSent = counter.BytesSent
			info.PacketsReceived = counter.PacketsRecv
			info.PacketsSent = counter.PacketsSent
			info.ErrorsIn = counter.Errin
			info.ErrorsOut = counter.Errout
			info.DropsIn = counter.Dropin
			info.DropsOut = counter.Dropout
		}

		result = append(result, info)
	}

	return result, nil
}

// InterfaceInfo holds detailed information about a network interface
type InterfaceInfo struct {
	Index           int      // Interface index
	MTU             int      // Maximum transmission unit
	Name            string   // Interface name (e.g., "eth0", "en0")
	HardwareAddr    string   // MAC address
	Flags           []string // Interface flags (e.g., "up", "broadcast")
	Addresses       []string // IP addresses assigned to this interface
	BytesReceived   uint64   // Total bytes received
	BytesSent       uint64   // Total bytes sent
	PacketsReceived uint64   // Total packets received
	PacketsSent     uint64   // Total packets sent
	ErrorsIn        uint64   // Input errors
	ErrorsOut       uint64   // Output errors
	DropsIn         uint64   // Input drops
	DropsOut        uint64   // Output drops
}

// IsLoopback returns true if this interface is a loopback interface
func (i *InterfaceInfo) IsLoopback() bool {
	for _, flag := range i.Flags {
		if strings.ToLower(flag) == "loopback" {
			return true
		}
	}
	return false
}

// IsUp returns true if this interface is up
func (i *InterfaceInfo) IsUp() bool {
	for _, flag := range i.Flags {
		if strings.ToLower(flag) == "up" {
			return true
		}
	}
	return false
}

// GetNetworkConnections returns active network connections
// This can be filtered by connection kind (tcp, udp, etc.)
func GetNetworkConnections(ctx context.Context, kind string) ([]ConnectionInfo, error) {
	connections, err := net.ConnectionsWithContext(ctx, kind)
	if err != nil {
		return nil, fmt.Errorf("failed to get network connections: %w", err)
	}

	result := make([]ConnectionInfo, 0, len(connections))
	for _, conn := range connections {
		result = append(result, ConnectionInfo{
			FD:     conn.Fd,
			Family: conn.Family,
			Type:   conn.Type,
			LocalAddr: Address{
				IP:   conn.Laddr.IP,
				Port: conn.Laddr.Port,
			},
			RemoteAddr: Address{
				IP:   conn.Raddr.IP,
				Port: conn.Raddr.Port,
			},
			Status: conn.Status,
			PID:    conn.Pid,
		})
	}

	return result, nil
}

// ConnectionInfo holds information about a network connection
type ConnectionInfo struct {
	FD         uint32  // File descriptor
	Family     uint32  // Address family (AF_INET, AF_INET6)
	Type       uint32  // Socket type (SOCK_STREAM, SOCK_DGRAM)
	LocalAddr  Address // Local address
	RemoteAddr Address // Remote address
	Status     string  // Connection status (ESTABLISHED, LISTEN, etc.)
	PID        int32   // Process ID that owns this connection
}

// Address represents an IP address and port
type Address struct {
	IP   string
	Port uint32
}

// NetworkRateMonitor calculates network transfer rates
// Tracks bytes/sec and packets/sec by diffing cumulative counters
type NetworkRateMonitor struct {
	lastStats      map[string]net.IOCountersStat
	lastSampleTime int64 // Unix timestamp in nanoseconds
	mu             sync.RWMutex
}

// NewNetworkRateMonitor creates a new network rate monitor
func NewNetworkRateMonitor() *NetworkRateMonitor {
	return &NetworkRateMonitor{
		lastStats: make(map[string]net.IOCountersStat),
	}
}

// Update captures current network stats and returns rates since last update
// Returns nil on first call (no previous data to diff against)
// The interval parameter specifies the time between samples for rate calculation
func (m *NetworkRateMonitor) Update(ctx context.Context) (map[string]*NetworkRates, error) {
	currentStats, err := net.IOCountersWithContext(ctx, true) // pernic=true
	if err != nil {
		return nil, fmt.Errorf("failed to get network stats: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// First call - just store stats and return nil
	if len(m.lastStats) == 0 {
		for _, stat := range currentStats {
			m.lastStats[stat.Name] = stat
		}
		return nil, nil
	}

	// Calculate rates
	rates := make(map[string]*NetworkRates)
	for _, current := range currentStats {
		last, exists := m.lastStats[current.Name]
		if !exists {
			// New interface appeared, skip for now
			continue
		}

		rates[current.Name] = &NetworkRates{
			Name:             current.Name,
			BytesRecvDelta:   safeDelta(current.BytesRecv, last.BytesRecv),
			BytesSentDelta:   safeDelta(current.BytesSent, last.BytesSent),
			PacketsRecvDelta: safeDelta(current.PacketsRecv, last.PacketsRecv),
			PacketsSentDelta: safeDelta(current.PacketsSent, last.PacketsSent),
			ErrorsInDelta:    safeDelta(current.Errin, last.Errin),
			ErrorsOutDelta:   safeDelta(current.Errout, last.Errout),
			DropsInDelta:     safeDelta(current.Dropin, last.Dropin),
			DropsOutDelta:    safeDelta(current.Dropout, last.Dropout),
		}
	}

	// Store current stats for next diff
	for _, stat := range currentStats {
		m.lastStats[stat.Name] = stat
	}

	return rates, nil
}

// safeDelta calculates the delta between two uint64 values
// Handles counter wraps by returning 0 instead of negative values
func safeDelta(current, last uint64) uint64 {
	if current >= last {
		return current - last
	}
	// Counter wrapped or reset, return current value
	return current
}

// NetworkRates holds network transfer rates between two samples
type NetworkRates struct {
	Name             string // Interface name
	BytesRecvDelta   uint64 // Bytes received since last sample
	BytesSentDelta   uint64 // Bytes sent since last sample
	PacketsRecvDelta uint64 // Packets received since last sample
	PacketsSentDelta uint64 // Packets sent since last sample
	ErrorsInDelta    uint64 // Input errors since last sample
	ErrorsOutDelta   uint64 // Output errors since last sample
	DropsInDelta     uint64 // Input drops since last sample
	DropsOutDelta    uint64 // Output drops since last sample
}
