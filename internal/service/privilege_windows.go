//go:build windows

package service

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

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
	elevation := windows.TOKEN_ELEVATION{}
	var returnedLen uint32
	err = windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevation)), uint32(unsafe.Sizeof(elevation)), &returnedLen)
	if err != nil {
		return false
	}

	return elevation.TokenIsElevated != 0
}
