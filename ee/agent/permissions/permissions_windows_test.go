//go:build windows

package permissions

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestRestrictFileAccessToRootOnly(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	rootDir := t.TempDir()
	testFile := filepath.Join(rootDir, "test.txt")
	fh, err := os.Create(testFile)
	require.NoError(t, err)
	require.NoError(t, fh.Close())

	require.NoError(t, RestrictFileAccessToRootOnly(testFile))

	// Check permissions on the file path match what we expect -- get the security info for the file
	fileSecurityInfo, err := windows.GetNamedSecurityInfo(testFile, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err, "getting named security info for file")
	require.True(t, fileSecurityInfo.IsValid())
	fileDacl, _, err := fileSecurityInfo.DACL()
	require.NoError(t, err, "getting DACL for file")
	require.NotNil(t, fileDacl)

	// Confirm that users have not been granted any permissions -- get the allowed SIDs
	adminSid, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	require.NoError(t, err, "getting admin SID")
	creatorOwnerSid, err := windows.CreateWellKnownSid(windows.WinCreatorOwnerSid)
	require.NoError(t, err, "getting creator/owner SID")
	systemSid, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	require.NoError(t, err, "getting system SID")

	// Iterate through all access control entries and confirm they only apply to the allowed SIDs
	for i := 0; i < int(fileDacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		require.NoError(t, windows.GetAce(fileDacl, uint32(i), &ace), "getting ACE")
		sid := (*windows.SID)(unsafe.Pointer(uintptr(unsafe.Pointer(ace)) + unsafe.Offsetof(ace.SidStart)))
		require.True(t, sid.Equals(adminSid) || sid.Equals(creatorOwnerSid) || sid.Equals(systemSid), "invalid SID")
	}
}
