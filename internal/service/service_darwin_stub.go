//go:build !darwin

package service

import (
	"fmt"
)

// newDarwinService is not available on non-Darwin platforms
func newDarwinService(config *Config) (Service, error) {
	return nil, fmt.Errorf("darwin service not supported on this platform")
}
