//go:build !windows

package service

import (
	"fmt"
)

// newWindowsService is not available on non-Windows platforms
func newWindowsService(config *Config) (Service, error) {
	return nil, fmt.Errorf("windows service not supported on this platform")
}

// isElevatedWindows always returns false on non-Windows platforms
func isElevatedWindows() bool {
	return false
}
