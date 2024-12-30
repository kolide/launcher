//go:build windows
// +build windows

package runtime

import (
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"github.com/kolide/launcher/ee/agent/types"
	typesMocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func TestCreateOsqueryCommandEnvVars(t *testing.T) {
	t.Parallel()

	osquerydPath := testOsqueryBinary

	k := typesMocks.NewKnapsack(t)
	k.On("WatchdogEnabled").Return(true)
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)
	k.On("OsqueryVerbose").Return(true)
	k.On("OsqueryFlags").Return([]string{})
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("RootDirectory").Return("")

	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient())

	cmd, err := i.createOsquerydCommand(osquerydPath, &osqueryFilePaths{
		pidfilePath:           "/foo/bar/osquery-abcd.pid",
		databasePath:          "/foo/bar/osquery.db",
		extensionSocketPath:   "/foo/bar/osquery.sock",
		extensionAutoloadPath: "/foo/bar/osquery.autoload",
	})
	require.NoError(t, err)

	systemDriveEnvVarFound := false
	for _, envVar := range cmd.Env {
		if strings.Contains(envVar, "SystemDrive") {
			systemDriveEnvVarFound = true
			break
		}
	}

	require.True(t, systemDriveEnvVarFound, "SystemDrive env var missing from command")

	k.AssertExpectations(t)
}

func createLockFile(t *testing.T, fileToLock string) {
	lockedFileName, err := syscall.UTF16PtrFromString(fileToLock)
	require.NoError(t, err)

	handle, err := syscall.CreateFile(
		lockedFileName,
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.CREATE_ALWAYS,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		syscall.Close(handle)
	})

	// See: https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-lockfileex
	modkernel32 := syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx := modkernel32.NewProc("LockFileEx")
	overlapped := &syscall.Overlapped{
		HEvent: 0,
	}

	r1, _, e1 := syscall.SyscallN(
		procLockFileEx.Addr(),
		uintptr(handle),
		uintptr(2), // LOCKFILE_EXCLUSIVE_LOCK
		uintptr(0), // Reserved parameter; must be set to zero.
		uintptr(1), // nNumberOfBytesToLockLow -- the low-order 32 bits of the length of the byte range to lock.
		uintptr(0), // nNumberOfBytesToLockHigh -- the high-order 32 bits of the length of the byte range to lock.
		uintptr(unsafe.Pointer(overlapped)),
	)
	require.False(t, r1 == 0, error(e1))
}
