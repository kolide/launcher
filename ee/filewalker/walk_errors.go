package filewalker

import (
	"io/fs"
	"strconv"
	"syscall"
)

// classifyWalkError returns a consistent, platform-specific key for the provided error
// or "other" if it is not a fs.PathError. Known, expected errors when walking return true.
func classifyWalkError(err error) (key string, expected bool) {
	pathErr, ok := err.(*fs.PathError)
	if !ok {
		return "other", false
	}
	errno, ok := pathErr.Err.(syscall.Errno)
	if !ok {
		return pathErr.Op + ":other", false
	}

	key = pathErr.Op + ":" + strconv.FormatUint(uint64(errno), 10)
	switch {
	case errno.Is(fs.ErrPermission), errno.Is(fs.ErrNotExist):
		return key, true
	case errno == syscall.ENOTDIR, errno == syscall.EDEADLK:
		return key, true
	}
	return key, isExpectedPlatformErrno(errno)
}
