//go:build windows
// +build windows

package main

import (
	"log/slog"
	"os"
	"path/filepath"
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

func Test_checkEnrollSecretACLs(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	secretPath := filepath.Join(confDir, "secret")
	require.NoError(t, os.WriteFile(secretPath, []byte("secretsecretshhh"), 0644))

	// Get info about our starting permissions
	intialSecretInfo, err := windows.GetNamedSecurityInfo(secretPath, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err, "getting initial named security info")
	initialSecretDacl, _, err := intialSecretInfo.DACL()
	require.NoError(t, err, "getting initial DACL")
	require.NotNil(t, initialSecretDacl)

	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Check the secret ACLs -- expect that we update the permissions
	checkEnrollSecretACLs(slogger, secretPath)
	require.Contains(t, logBytes.String(), "updated ACLs for enroll secret")

	// Get our updated permissions
	updatedSecretInfo, err := windows.GetNamedSecurityInfo(secretPath, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err, "getting secret named security info")
	updatedSecretDacl, _, err := updatedSecretInfo.DACL()
	require.NoError(t, err, "getting secret DACL")
	require.NotNil(t, updatedSecretDacl)

	// Confirm permissions have updated
	require.NotEqual(t, intialSecretInfo.String(), updatedSecretInfo.String(), "permissions did not change")

	// Confirm that users have not been granted any permissions
	usersSID, err := windows.CreateWellKnownSid(windows.WinBuiltinUsersSid)
	require.NoError(t, err, "getting users SID")
	for i := 0; i < int(updatedSecretDacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		require.NoError(t, windows.GetAce(updatedSecretDacl, uint32(i), &ace), "getting ACE")

		sid := (*windows.SID)(unsafe.Pointer(uintptr(unsafe.Pointer(ace)) + unsafe.Offsetof(ace.SidStart)))
		require.False(t, sid.Equals(usersSID), "did not expect to find ACE for users SID")
	}

	// Run checkEnrollSecretACLs and confirm that the permissions do not change
	checkEnrollSecretACLs(slogger, secretPath)

	// Get permissions again
	secretInfoFinal, err := windows.GetNamedSecurityInfo(secretPath, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err, "getting secret named security info after running permissions update twice")
	secretDaclFinal, _, err := secretInfoFinal.DACL()
	require.NoError(t, err, "getting secret DACL after running permissions update twice")
	require.NotNil(t, secretDaclFinal)

	// Confirm permissions have not updated
	require.Equal(t, updatedSecretInfo.String(), secretInfoFinal.String(), "permissions should not have changed")
}
