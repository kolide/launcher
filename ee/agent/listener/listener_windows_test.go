//go:build windows

package listener

import (
	"errors"
	"log/slog"
	"testing"
	"unsafe"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// TestPermissions confirms that the socket file is created with appropriately-restricted permissions.
func TestPermissions(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	rootDir := t.TempDir()
	testPrefix := "test"
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(rootDir).Maybe()

	// Set up listener
	testListener, err := NewLauncherListener(mockKnapsack, slogger, testPrefix)
	require.NoError(t, err)
	require.NotNil(t, testListener.listener)
	t.Cleanup(func() { testListener.Interrupt(errors.New("test error")) })

	// Check permissions on socket path match what we expect -- get the security info for the socket
	socketSecurityInfo, err := windows.GetNamedSecurityInfo(testListener.socketPath, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err, "getting named security info for socket")
	require.True(t, socketSecurityInfo.IsValid())
	socketDacl, _, err := socketSecurityInfo.DACL()
	require.NoError(t, err, "getting DACL for socket")
	require.NotNil(t, socketDacl)

	// Confirm that users have not been granted any permissions -- get the allowed SIDs
	adminSid, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	require.NoError(t, err, "getting admin SID")
	creatorOwnerSid, err := windows.CreateWellKnownSid(windows.WinCreatorOwnerSid)
	require.NoError(t, err, "getting creator/owner SID")
	systemSid, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	require.NoError(t, err, "getting system SID")

	// Iterate through all access control entries and confirm they only apply to the allowed SIDs
	for i := 0; i < int(socketDacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		require.NoError(t, windows.GetAce(socketDacl, uint32(i), &ace), "getting ACE")
		sid := (*windows.SID)(unsafe.Pointer(uintptr(unsafe.Pointer(ace)) + unsafe.Offsetof(ace.SidStart)))
		require.True(t, sid.Equals(adminSid) || sid.Equals(creatorOwnerSid) || sid.Equals(systemSid), "invalid SID")
	}
}
