//go:build unix || darwin || linux

package cache

import (
	"os"
)

// ensureDirImpl creates a directory if it doesn't exist (Unix implementation)
func ensureDirImpl(dir string) error {
	return os.MkdirAll(dir, 0755)
}
