package packagemgr

import (
	"testing"
)

func TestPackageError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *PackageError
		expected string
	}{
		{
			name: "simple error",
			err: &PackageError{
				Code:    "TEST_ERROR",
				Message: "test error message",
			},
			expected: "test error message",
		},
		{
			name: "wrapped error",
			err: &PackageError{
				Code:    "TEST_ERROR",
				Message: "test error message",
				Wrapped: ErrNotInstalled,
			},
			expected: "test error message: package manager not installed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("PackageError.Error() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestPackageError_Unwrap(t *testing.T) {
	wrapped := ErrNotInstalled
	err := &PackageError{
		Code:    "TEST_ERROR",
		Message: "test",
		Wrapped: wrapped,
	}

	if got := err.Unwrap(); got != wrapped {
		t.Errorf("PackageError.Unwrap() = %v, want %v", got, wrapped)
	}
}

func TestPackageError_WithDetails(t *testing.T) {
	err := &PackageError{
		Code:    "TEST_ERROR",
		Message: "test",
	}

	err.WithDetails("key1", "value1")
	err.WithDetails("key2", 123)

	if len(err.Details) != 2 {
		t.Errorf("Expected 2 details, got %d", len(err.Details))
	}

	if err.Details["key1"] != "value1" {
		t.Errorf("Expected key1=value1, got %v", err.Details["key1"])
	}

	if err.Details["key2"] != 123 {
		t.Errorf("Expected key2=123, got %v", err.Details["key2"])
	}
}

func TestWrapError(t *testing.T) {
	originalErr := ErrNotInstalled
	wrapped := WrapError(originalErr, "CUSTOM_CODE", "custom message")

	if wrapped.Code != "CUSTOM_CODE" {
		t.Errorf("Expected code CUSTOM_CODE, got %s", wrapped.Code)
	}

	if wrapped.Message != "custom message" {
		t.Errorf("Expected message 'custom message', got %s", wrapped.Message)
	}

	if wrapped.Wrapped != originalErr {
		t.Errorf("Expected wrapped error to be originalErr")
	}
}

func TestPackage_DefaultValues(t *testing.T) {
	pkg := &Package{
		Name: "test-package",
	}

	if pkg.Name != "test-package" {
		t.Errorf("Expected name 'test-package', got %s", pkg.Name)
	}

	if pkg.Installed {
		t.Errorf("Expected Installed to be false by default")
	}

	if pkg.Version != "" {
		t.Errorf("Expected Version to be empty by default")
	}
}

func BenchmarkPackageError_Error(b *testing.B) {
	err := &PackageError{
		Code:    "BENCHMARK_ERROR",
		Message: "this is a benchmark error message",
		Wrapped: ErrNotInstalled,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = err.Error()
	}
}

func BenchmarkPackageError_WithDetails(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := &PackageError{
			Code:    "BENCHMARK_ERROR",
			Message: "benchmark",
		}
		err.WithDetails("key", "value")
	}
}
