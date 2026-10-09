//go:build windows

package filewalker

import (
	"syscall"

	"golang.org/x/sys/windows"
)

func isExpectedPlatformErrno(errno syscall.Errno) bool {
	return errno == windows.ERROR_SHARING_VIOLATION || errno == windows.ERROR_CLOUD_FILE_METADATA_CORRUPT
}
