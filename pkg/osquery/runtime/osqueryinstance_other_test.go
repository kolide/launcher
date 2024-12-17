//go:build !windows
// +build !windows

package runtime

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func createLockFile(t *testing.T, fileToLock string) {
	lockFile, err := syscall.Open(fileToLock, syscall.O_CREAT|syscall.O_RDONLY, 0600)
	require.NoError(t, err)
	require.NoError(t, syscall.Flock(lockFile, syscall.LOCK_EX|syscall.LOCK_NB))
	t.Cleanup(func() {
		syscall.Close(lockFile)
	})
}
