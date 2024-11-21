//go:build windows
// +build windows

package main

import (
	"log/slog"
	"testing"
	"unsafe"

	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func Test_checkRootDirACLs(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()

	// Get info about our starting permissions
	initialRootDirInfo, err := windows.GetNamedSecurityInfo(rootDir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err, "getting named security info")
	initialRootDirDacl, _, err := initialRootDirInfo.DACL()
	require.NoError(t, err, "getting DACL")
	require.NotNil(t, initialRootDirDacl)

	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Check the root dir ACLs -- expect that we update the permissions
	checkRootDirACLs(slogger, rootDir)
	require.Contains(t, logBytes.String(), "updated ACLs for root directory")

	// Get our updated permissions
	rootDirInfo, err := windows.GetNamedSecurityInfo(rootDir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err, "getting named security info")
	rootDirDacl, _, err := rootDirInfo.DACL()
	require.NoError(t, err, "getting DACL")
	require.NotNil(t, rootDirDacl)

	// Confirm permissions have updated
	require.NotEqual(t, initialRootDirInfo.String(), rootDirInfo.String(), "permissions did not change")

	// Confirm that users only have access to read+execute
	usersSID, err := windows.CreateWellKnownSid(windows.WinBuiltinUsersSid)
	require.NoError(t, err, "getting users SID")
	userAceFound := false
	for i := 0; i < int(rootDirDacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		require.NoError(t, windows.GetAce(rootDirDacl, uint32(i), &ace), "getting ACE")

		if ace.Mask != windows.GENERIC_READ|windows.GENERIC_EXECUTE {
			continue
		}

		sid := (*windows.SID)(unsafe.Pointer(uintptr(unsafe.Pointer(ace)) + unsafe.Offsetof(ace.SidStart)))
		if sid.Equals(usersSID) {
			userAceFound = true
			break
		}
	}
	require.True(t, userAceFound, "ACE not found for WinBuiltinUsersSid with permissions GENERIC_READ|GENERIC_EXECUTE")

	// Run checkRootDirACLs and confirm that the permissions do not change
	checkRootDirACLs(slogger, rootDir)

	// Get permissions again
	rootDirInfoUpdated, err := windows.GetNamedSecurityInfo(rootDir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err, "getting named security info")
	rootDirDaclUpdated, _, err := rootDirInfoUpdated.DACL()
	require.NoError(t, err, "getting DACL")
	require.NotNil(t, rootDirDaclUpdated)

	// Confirm permissions have not updated
	require.Equal(t, rootDirInfo.String(), rootDirInfoUpdated.String(), "permissions should not have changed")
}
