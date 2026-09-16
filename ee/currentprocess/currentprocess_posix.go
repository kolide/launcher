//go:build !windows

package currentprocess

import (
	"os"
	"strconv"
)

// Returns whether the current process is root.
func IsElevated() (bool, error) {
	return os.Geteuid() == 0, nil
}

// Returns the current process's numerical user id. All platforms
// return strings because of Windows.
func Uid() (string, error) {
	return strconv.Itoa(os.Getuid()), nil
}
