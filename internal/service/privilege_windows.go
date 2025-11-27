//go:build windows

package service

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// tokenElevation is the TOKEN_ELEVATION structure for Windows API
// This struct is not exported by golang.org/x/sys/windows, so we define it here
type tokenElevation struct {
	TokenIsElevated uint32
}

// isElevatedWindows checks if running with administrator privileges on Windows
func isElevatedWindows() bool {
	// Get current process token
	var token windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token)
	if err != nil {
		return false
	}
	defer token.Close()

	// Check if token is elevated
	var elevation tokenElevation
	var returnedLen uint32
	err = windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevation)), uint32(unsafe.Sizeof(elevation)), &returnedLen)
	if err != nil {
		return false
	}

	return elevation.TokenIsElevated != 0
}

// isElevated is the platform-specific implementation for Windows
func isElevated() bool {
	return isElevatedWindows()
}
