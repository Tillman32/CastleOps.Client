# CastleOps.Client

A high-performance, lightweight Go-based desktop agent for macOS and Windows that runs as a background service with offline-first capabilities. Monitors system metrics and manages software installations via a remote REST API.

## Features

- **Lightweight & Fast**: Minimal CPU/memory footprint (target: <50MB RAM, <1% CPU idle)
- **Offline-First**: Local SQLite caching with automatic sync when connectivity is restored
- **Cross-Platform**: Native support for macOS and Windows with platform-specific service integration
- **System Metrics**: Collects CPU, memory, disk, and network metrics
- **Package Management**: Extensible package manager abstraction (Chocolatey, Homebrew)
- **Secure Communication**: TLS/mTLS support for API communication
- **Graceful Shutdown**: Proper cleanup and data flushing on termination

## Architecture

The project follows a clean, interface-based architecture optimized for performance:

```
CastleOps.Client/
├── cmd/castleops-client/     # Application entry point
├── internal/
│   ├── agent/                # Agent orchestrator and core logic
│   ├── api/                  # REST API client with retry logic
│   ├── cache/                # SQLite and in-memory cache implementations
│   ├── config/               # Configuration management (viper)
│   ├── metrics/              # System metrics collection (gopsutil)
│   ├── packagemgr/           # Package manager abstraction
│   └── service/              # Service/daemon integration
├── pkg/protocol/             # Shared protocol definitions
└── configs/                  # Configuration files
```

## Performance Design

### Zero-Allocation Patterns
- Preallocated buffers and object pooling (`sync.Pool`)
- Stack allocation preference over heap
- Efficient buffer reuse for I/O operations

### Concurrency Optimization
- Goroutine pools to prevent unbounded goroutine creation
- Non-blocking metric collection with context cancellation
- Lock-free operations where possible using atomics

### Resource Efficiency
- HTTP connection pooling and keep-alive
- Batched metric transmission (reduces syscalls)
- SQLite WAL mode for concurrent read/write
- Configurable retention policies for automatic cleanup

## Quick Start

### Prerequisites

- Go 1.25.4 or later
- GCC (for SQLite CGO compilation)
  - macOS: `xcode-select --install`
  - Windows: MinGW-w64 for cross-compilation

### Building

```bash
# Build for current platform
make build

# Build for specific platforms
make build-darwin   # macOS (both amd64 and arm64)
make build-windows  # Windows (requires mingw-w64)
make build-linux    # Linux

# Build all release binaries
make release

# Run tests
make test

# Run benchmarks
make bench
```

### Running

```bash
# Run with default configuration
./build/castleops-client

# Run with custom configuration file
./build/castleops-client -config /path/to/config.yaml

# Display version
./build/castleops-client -version

# Install as system service
sudo ./build/castleops-client -install  # macOS/Linux
# Or on Windows (as Administrator):
.\build\castleops-client.exe -install
```

### Configuration

Copy `configs/config.yaml.example` to `config.yaml` and customize:

```yaml
server:
  url: "https://api.castleops.com"
  tls_verify: true
  timeout: 30s

heartbeat:
  interval: 30s
  retry_attempts: 3

metrics:
  collection_interval: 60s
  batch_size: 100
  retention_days: 7

cache:
  type: "sqlite"
  path: ""  # Uses platform default if empty

logging:
  level: "info"
  format: "json"
```

Configuration can be overridden with environment variables:

```bash
export CASTLEOPS_SERVER_URL="https://custom-api.example.com"
export CASTLEOPS_LOGGING_LEVEL="debug"
export CASTLEOPS_METRICS_COLLECTION_INTERVAL="30s"
```

## Development

### Project Status

**Current Status**: All core components implemented, QA validation complete

Implementation status:

- [x] Project structure and build system
- [x] Configuration management with viper
- [x] Main entry point with signal handling
- [x] Cache layer (SQLite/memory) - 68.8% test coverage
- [x] Metrics collection - 63.3% test coverage
- [x] API client with retry logic - 4 tests need fixes
- [x] Agent orchestrator - needs comprehensive tests
- [x] Heartbeat service
- [x] Package manager implementations (Chocolatey, Homebrew) - needs comprehensive tests
- [x] Service/daemon integration (macOS launchd, Windows SCM)

**Quality Status**:
- 127 tests total (119 passing, 4 failing)
- 51.8% overall test coverage
- Zero race conditions detected
- Production readiness: 56% (see QA_REPORT.md for details)

**Next Steps**: Fix 4 failing API tests and expand test coverage for agent and package manager components (3-4 weeks to production-ready)

### Development Workflow

```bash
# Format code
make fmt

# Run static analysis
make vet

# Run linter (requires golangci-lint)
make lint

# Run all checks (format, vet, test)
make check

# Development mode (format, vet, build, run)
make dev

# Profile CPU usage
make bench-cpu

# Profile memory usage
make bench-mem
```

### Testing

```bash
# Run all tests with race detection
make test

# Run short tests (faster, no race detection)
make test-short

# Run benchmarks
make bench
```

## Dependencies

Core dependencies are carefully selected for performance and reliability:

- **viper**: Flexible configuration management
- **zerolog**: High-performance structured logging
- **gopsutil**: Cross-platform system metrics
- **go-sqlite3**: Embedded SQLite database
- **kardianos/service**: Service/daemon management

## Performance Targets

- Memory usage: <50MB RAM during normal operation
- CPU usage: <1% CPU when idle
- Startup time: <1 second
- Shutdown time: <5 seconds (graceful)
- API response time: <100ms (local operations)

## Platform Support

### macOS
- Service integration via launchd
- Native package management via Homebrew
- Code signing support

### Windows
- Service integration via Windows Service Manager
- Native package management via Chocolatey
- Event log integration

### Linux (Future)
- Service integration via systemd
- Package management via apt/yum/snap

## Security

- TLS 1.3 enforced for API communication
- Token-based authentication
- Secure credential storage (OS keychain integration planned)
- Input validation and sanitization
- Code signing for distributed binaries

## License

Copyright (c) 2024 CastleOps. All rights reserved.

## Contributing

This is a private project. For questions or contributions, please contact the project maintainers.

## Roadmap

See [plan.md](plan.md) for detailed architecture and implementation roadmap.

### Phase 1: Core Infrastructure ✅ COMPLETE
- [x] Project initialization
- [x] Configuration management
- [x] Logging framework
- [x] Service/daemon wrappers

### Phase 2: Communication Layer ✅ COMPLETE
- [x] REST API client
- [x] Registration flow
- [x] Heartbeat service
- [x] Command polling

### Phase 3: Caching & Data ✅ COMPLETE
- [x] SQLite schema
- [x] Cache interface implementation
- [x] Offline queue
- [x] Retention policies

### Phase 4: Metrics Collection ✅ COMPLETE
- [x] gopsutil integration
- [x] CPU/memory/disk/network collectors
- [x] Metric aggregation
- [x] Batching mechanism

### Phase 5: Package Management ✅ COMPLETE
- [x] Package manager interface
- [x] Chocolatey support
- [x] Homebrew support
- [x] Registry/factory pattern

### Phase 6: Advanced Features (Future)
- [ ] UDP transport
- [ ] WebSocket support
- [ ] Compression
- [x] TLS (mTLS support ready)

### Phase 7: Production Readiness (In Progress)
- [x] Comprehensive error handling
- [x] Integration tests (partial - 4 failing)
- [x] Performance benchmarks
- [x] Installation scripts
- [x] Documentation
- [ ] Fix failing API tests
- [ ] Expand agent test coverage
- [ ] Expand package manager test coverage
- [ ] End-to-end integration tests
- [ ] CI/CD pipeline
