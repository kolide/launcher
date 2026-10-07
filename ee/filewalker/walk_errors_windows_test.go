//go:build windows

package filewalker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func setupExpectedWalkErrors(t *testing.T, root string) {
	// ERROR_SHARING_VIOLATION: hold the dir open with no sharing, as AV/backup tools do.
	locked := filepath.Join(root, "locked")
	require.NoError(t, os.Mkdir(locked, 0o755))

	lockedPtr, err := windows.UTF16PtrFromString(locked)
	require.NoError(t, err)

	h, err := windows.CreateFile(lockedPtr, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = windows.CloseHandle(h) })

	// ERROR_ACCESS_DENIED: a protected, empty DACL. The owner keeps implicit WRITE_DAC to restore it.
	denied := filepath.Join(root, "denied")
	require.NoError(t, os.Mkdir(denied, 0o755))

	orig, err := windows.GetNamedSecurityInfo(denied, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)

	origDACL, _, err := orig.DACL()
	require.NoError(t, err)

	empty, err := windows.SecurityDescriptorFromString("D:P")
	require.NoError(t, err)

	emptyDACL, _, err := empty.DACL()
	require.NoError(t, err)
	require.NoError(t, windows.SetNamedSecurityInfo(denied, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, emptyDACL, nil))

	t.Cleanup(func() {
		_ = windows.SetNamedSecurityInfo(denied, windows.SE_FILE_OBJECT,
			windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION, nil, nil, origDACL, nil)
	})
}
