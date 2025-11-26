//go:build darwin || linux

package service

import (
	"os"
)

// isElevatedUnix checks if running with root privileges on Unix-like systems
func isElevatedUnix() bool {
	return os.Geteuid() == 0
}
