package service

import (
	"os"
	"runtime"
	"testing"

	"github.com/rs/zerolog"
)

// testLogger creates a no-op logger for testing
func testLogger() zerolog.Logger {
	return zerolog.New(os.Stderr).Level(zerolog.Disabled)
}

// TestNewServiceWithValidConfig tests service creation with valid configuration
func TestNewServiceWithValidConfig(t *testing.T) {
	cfg := &Config{
		Name:             "test-service",
		DisplayName:      "Test Service",
		Description:      "Test service for unit testing",
		Executable:       "/usr/bin/test",
		WorkingDirectory: "/tmp",
		Logger:           testLogger(),
	}

	svc, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v, wantErr false", err)
	}

	if svc == nil {
		t.Fatal("New() returned nil")
	}

	// Verify basic properties
	if svc.Name() != "test-service" {
		t.Errorf("Name() = %q, want %q", svc.Name(), "test-service")
	}

	if svc.DisplayName() != "Test Service" {
		t.Errorf("DisplayName() = %q, want %q", svc.DisplayName(), "Test Service")
	}
}

// TestNewServiceWithInvalidConfig tests service creation with invalid config
func TestNewServiceWithInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name: "missing name",
			config: &Config{
				DisplayName: "Test Service",
				Executable:  "/usr/bin/test",
				Logger:      testLogger(),
			},
		},
		{
			name: "missing display name",
			config: &Config{
				Name:       "test-service",
				Executable: "/usr/bin/test",
				Logger:     testLogger(),
			},
		},
		{
			name: "missing executable",
			config: &Config{
				Name:        "test-service",
				DisplayName: "Test Service",
				Logger:      testLogger(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.config)
			if err == nil {
				t.Errorf("New() should fail for config with %s", tt.name)
			}
		})
	}
}

// TestConfigValidation tests the Config.Validate() method
func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Name:        "test-service",
				DisplayName: "Test Service",
				Executable:  "/usr/bin/test",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			config: &Config{
				DisplayName: "Test Service",
				Executable:  "/usr/bin/test",
			},
			wantErr: true,
		},
		{
			name: "missing display name",
			config: &Config{
				Name:       "test-service",
				Executable: "/usr/bin/test",
			},
			wantErr: true,
		},
		{
			name: "missing executable",
			config: &Config{
				Name:        "test-service",
				DisplayName: "Test Service",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestStatusString tests the Status.String() method
func TestStatusString(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusUnknown, "unknown"},
		{StatusNotInstalled, "not installed"},
		{StatusStopped, "stopped"},
		{StatusStarting, "starting"},
		{StatusRunning, "running"},
		{StatusStopping, "stopping"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestIsElevated tests privilege detection
func TestIsElevated(t *testing.T) {
	// This test mainly ensures the function doesn't panic
	// Actual privilege level depends on how tests are run
	elevated := IsElevated()

	// Should return a boolean without error
	if elevated && os.Getuid() != 0 && runtime.GOOS != "windows" {
		t.Logf("IsElevated() returned true but getuid != 0: may be in container or special environment")
	}
}

// TestRequireElevated tests privilege requirement validation
func TestRequireElevated(t *testing.T) {
	err := RequireElevated()

	if IsElevated() && err != nil {
		t.Errorf("RequireElevated() returned error when elevated: %v", err)
	}

	if !IsElevated() && err == nil {
		t.Error("RequireElevated() should return error when not elevated")
	}
}

// TestServiceConfiguration tests service config setup
func TestServiceConfiguration(t *testing.T) {
	cfg := &Config{
		Name:             "test.service",
		DisplayName:      "Test Service",
		Description:      "A test service",
		Executable:       "/usr/local/bin/test",
		Arguments:        []string{"-config", "/etc/test.conf"},
		WorkingDirectory: "/tmp",
		Dependencies:     []string{"dep1", "dep2"},
		UserService:      false,
		Logger:           testLogger(),
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if cfg.Name != "test.service" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test.service")
	}

	if cfg.DisplayName != "Test Service" {
		t.Errorf("DisplayName = %q, want %q", cfg.DisplayName, "Test Service")
	}

	if cfg.Description != "A test service" {
		t.Errorf("Description = %q, want %q", cfg.Description, "A test service")
	}

	if len(cfg.Arguments) != 2 {
		t.Errorf("Arguments length = %d, want 2", len(cfg.Arguments))
	}

	if len(cfg.Dependencies) != 2 {
		t.Errorf("Dependencies length = %d, want 2", len(cfg.Dependencies))
	}

	if cfg.UserService {
		t.Error("UserService should be false")
	}
}

// TestPlatformSpecificCreation tests that the correct platform implementation is used
func TestPlatformSpecificCreation(t *testing.T) {
	cfg := &Config{
		Name:       "test-service",
		DisplayName: "Test Service",
		Executable: "/usr/bin/test",
		Logger:     testLogger(),
	}

	svc, err := New(cfg)

	// Should succeed on supported platforms (darwin, windows)
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		if err != nil {
			t.Fatalf("New() should succeed on %s, got error: %v", runtime.GOOS, err)
		}
		if svc == nil {
			t.Fatal("New() returned nil on supported platform")
		}
	} else if runtime.GOOS == "linux" {
		// Linux support not yet implemented
		if err == nil {
			t.Errorf("New() should fail on linux (not yet supported), got nil error")
		}
	}
}

// TestServiceWithDependencies tests service with dependencies
func TestServiceWithDependencies(t *testing.T) {
	cfg := &Config{
		Name:             "dependent-service",
		DisplayName:      "Dependent Service",
		Description:      "Service with dependencies",
		Executable:       "/usr/bin/test",
		Dependencies:     []string{"network", "storage"},
		WorkingDirectory: "/tmp",
		Logger:           testLogger(),
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if len(cfg.Dependencies) != 2 {
		t.Errorf("Dependencies length = %d, want 2", len(cfg.Dependencies))
	}

	if cfg.Dependencies[0] != "network" {
		t.Errorf("Dependencies[0] = %q, want %q", cfg.Dependencies[0], "network")
	}
}

// TestServiceArguments tests service with command-line arguments
func TestServiceArguments(t *testing.T) {
	cfg := &Config{
		Name:        "arg-test-service",
		DisplayName: "Argument Test Service",
		Executable:  "/usr/bin/test",
		Arguments: []string{
			"-config", "/etc/test.conf",
			"-debug",
			"-timeout", "30s",
		},
		Logger: testLogger(),
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if len(cfg.Arguments) != 5 {
		t.Errorf("Arguments length = %d, want 5", len(cfg.Arguments))
	}

	if cfg.Arguments[0] != "-config" {
		t.Errorf("Arguments[0] = %q, want %q", cfg.Arguments[0], "-config")
	}
}

// TestNewServiceCreatesCorrectType tests type assertion for platform-specific services
func TestNewServiceCreatesCorrectType(t *testing.T) {
	cfg := &Config{
		Name:       "type-test-service",
		DisplayName: "Type Test Service",
		Executable: "/usr/bin/test",
		Logger:     testLogger(),
	}

	svc, err := New(cfg)

	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		if err != nil {
			t.Fatalf("New() error = %v on %s", err, runtime.GOOS)
		}

		// Service should implement the Service interface
		if svc == nil {
			t.Fatal("Service is nil")
		}

		// All required methods should be callable
		_ = svc.Name()
		_ = svc.DisplayName()
	}
}

// BenchmarkNewService benchmarks service creation
func BenchmarkNewService(b *testing.B) {
	cfg := &Config{
		Name:       "bench-service",
		DisplayName: "Benchmark Service",
		Executable: "/usr/bin/test",
		Logger:     testLogger(),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = New(cfg)
	}
}

// BenchmarkIsElevated benchmarks privilege checking
func BenchmarkIsElevated(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = IsElevated()
	}
}
