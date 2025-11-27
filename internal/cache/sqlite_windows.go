//go:build windows

package cache

import (
	"os"
)

// ensureDirImpl creates a directory if it doesn't exist (Windows implementation)
func ensureDirImpl(dir string) error {
	return os.MkdirAll(dir, 0755)
}
