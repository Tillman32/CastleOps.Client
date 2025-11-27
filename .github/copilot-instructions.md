# Copilot Instructions for CastleOps.Client

## Project Overview

CastleOps.Client is a high-performance, lightweight Go-based desktop agent for macOS and Windows. It runs as a background service with offline-first capabilities, monitors system metrics, and manages software installations via a remote REST API.

## Tech Stack

- **Language**: Go 1.25.4
- **CGO**: Required for SQLite3 (`github.com/mattn/go-sqlite3`)
- **Logging**: zerolog (`github.com/rs/zerolog`)
- **Configuration**: viper (`github.com/spf13/viper`)
- **Metrics**: gopsutil (`github.com/shirou/gopsutil/v4`)
- **Platforms**: macOS (launchd), Windows (SCM), Linux (systemd)

## Building and Testing

### Prerequisites

- Go 1.25.4 or later
- GCC (for SQLite CGO compilation)
  - macOS: `xcode-select --install`
  - Linux: `apt-get install gcc`
  - Windows: MinGW-w64

### Common Commands

```bash
# Download dependencies
make deps

# Build for current platform
make build

# Run all tests with race detection
make test

# Run short tests without race detection
make test-short

# Format code
make fmt

# Run go vet
make vet

# Run all checks (format, vet, test)
make check

# Run benchmarks
make bench
```

### Platform-Specific Builds

```bash
make build-darwin   # macOS (amd64 and arm64)
make build-windows  # Windows (requires mingw-w64)
make build-linux    # Linux
```

## Project Structure

```
CastleOps.Client/
├── cmd/castleops-client/     # Application entry point
├── internal/
│   ├── agent/                # Agent orchestrator and core logic
│   ├── api/                  # REST API client with retry logic
│   ├── cache/                # SQLite and in-memory cache implementations
│   ├── config/               # Configuration management (viper)
│   ├── metrics/              # System metrics collection (gopsutil)
│   ├── packagemgr/           # Package manager abstraction (Chocolatey, Homebrew)
│   └── service/              # Service/daemon integration
├── configs/                  # Configuration files
└── scripts/                  # Installation scripts
```

## Coding Standards

### Go Style

- Follow standard Go formatting (`gofmt`)
- Use `go vet` for static analysis
- Prefer interfaces over concrete types for dependencies
- Keep package imports organized: stdlib, external, internal

### Naming Conventions

- Use camelCase for unexported names, PascalCase for exported names
- Prefix interface names with their purpose (e.g., `Cache`, not `ICache`)
- Use descriptive names for functions and variables
- Constants should be descriptive (e.g., `DefaultHeartbeatInterval`)

### Error Handling

- Always wrap errors with context using `fmt.Errorf("context: %w", err)`
- Return errors rather than logging and continuing
- Use structured logging with zerolog for error reporting
- Include relevant context fields in log messages

### Performance Patterns

This project emphasizes performance. Follow these patterns:

1. **Zero-Allocation**: Use `sync.Pool` for frequently allocated objects
2. **Preallocated Buffers**: Reuse buffers for I/O operations
3. **Atomics**: Use `atomic.Bool`, `atomic.Int64` for lock-free state
4. **Context**: Always accept and respect `context.Context` for cancellation
5. **Goroutine Pools**: Prevent unbounded goroutine creation
6. **Connection Pooling**: Reuse HTTP connections

### Concurrency

- Use `sync.WaitGroup` for coordinating goroutines
- Use channels for communication, mutexes for shared state
- Always use `context.Context` for cancellation propagation
- Implement graceful shutdown with timeouts

### Testing

- Write table-driven tests where applicable
- Use `t.Helper()` for test helper functions
- Create test fixtures with helper functions (e.g., `testMetrics()`, `testLogger()`)
- Run tests with race detection: `go test -race ./...`
- Include benchmarks for performance-critical code

### Logging

Use zerolog with structured logging:

```go
logger.Info().
    Str("component", "agent").
    Dur("interval", interval).
    Msg("Component initialized")
```

## Configuration

Configuration is managed through viper with YAML files. Environment variables can override config values:

```bash
export CASTLEOPS_SERVER_URL="https://api.example.com"
export CASTLEOPS_LOGGING_LEVEL="debug"
```

## Important Notes

1. **CGO Dependency**: This project requires CGO for SQLite. Set `CGO_ENABLED=1` for builds.
2. **Cross-Platform**: Code must work on both macOS and Windows. Use build tags when necessary.
3. **Offline-First**: All operations should gracefully handle network failures.
4. **Resource Efficiency**: Target <50MB RAM, <1% CPU when idle.
5. **Graceful Shutdown**: Always implement proper cleanup with timeouts.

## CI/CD

The project uses GitHub Actions for CI:

- Tests run on push/PR to main
- Race detection is enabled for tests
- Builds are created for Linux, macOS (amd64/arm64), and Windows
