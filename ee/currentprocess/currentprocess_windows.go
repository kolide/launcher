//go:build windows

package currentprocess

import (
	"fmt"
	"os/user"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Detects UAC elevation or when running as LocalSystem.
// Impl is copied from windows.Token.IsElevated, but exposes the error
// on a failure to check.
func IsElevated() (bool, error) {
	var elevation uint32
	var outLen uint32
	if err := windows.GetTokenInformation(
		windows.GetCurrentProcessToken(),
		windows.TokenElevation,
		(*byte)(unsafe.Pointer(&elevation)),
		uint32(unsafe.Sizeof(elevation)),
		&outLen,
	); err != nil {
		return false, fmt.Errorf("failed to detect if process is running elevated: %w", err)
	}

	return outLen == uint32(unsafe.Sizeof(elevation)) && elevation != 0, nil
}

// Returns the current process's fully-qualified owner, DOMAIN\User.
func Uid() (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("getting current user: %w", err)
	}

	return currentUser.Username, nil
}
