//go:build windows

package main

import (
	"golang.org/x/sys/windows/svc"
)

// isWindowsService checks if the application is running as a Windows service
func isWindowsService() bool {
	isService, err := svc.IsWindowsService()
	return err == nil && isService
}
