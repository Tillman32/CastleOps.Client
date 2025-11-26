//go:build !windows

package main

// isWindowsService always returns false on non-Windows platforms
func isWindowsService() bool {
	return false
}
