//go:build !windows

package filewalker

import "syscall"

func isExpectedPlatformErrno(syscall.Errno) bool {
	return false
}
