//go:build windows

package listener

import (
	"fmt"
	"net"
	"strings"

	"github.com/Microsoft/go-winio"
	"github.com/kolide/kit/ulid"
	"golang.org/x/sys/windows"
)

const (
	FILE_CREATE_PIPE_INSTANCE = 0x00000004
	// duplexPipeAccessPermissions includes read, write, and sync permissions for sending data, and
	// FILE_CREATE_PIPE_INSTANCE to create the named pipe.
	// See: https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipe-security-and-access-rights
	duplexPipeAccessPermissions = windows.GENERIC_READ | windows.GENERIC_WRITE | windows.SYNCHRONIZE | FILE_CREATE_PIPE_INSTANCE
)

func (l *launcherListener) initPipe() (net.Listener, error) {
	// We only want root launcher and an admin-level user to be able to interact with the pipe --
	// get the relevant SIDs to create the appropriate access control policy for the pipe.
	creatorOwnerSID, err := windows.CreateWellKnownSid(windows.WinCreatorOwnerSid)
	if err != nil {
		return nil, fmt.Errorf("getting creator/owner SID: %w", err)
	}
	adminSid, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return nil, fmt.Errorf("getting admin SID: %w", err)
	}
	systemSid, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return nil, fmt.Errorf("getting system SID: %w", err)
	}

	// SYSTEM, admin, and creator/owner have full control; standard users are not granted any permissions.
	explicitAccessPolicies := []windows.EXPLICIT_ACCESS{
		{
			AccessPermissions: duplexPipeAccessPermissions,
			AccessMode:        windows.SET_ACCESS,
			Inheritance:       windows.NO_INHERITANCE,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(systemSid),
			},
		},
		{
			AccessPermissions: duplexPipeAccessPermissions,
			AccessMode:        windows.SET_ACCESS,
			Inheritance:       windows.NO_INHERITANCE,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(adminSid),
			},
		},
		{
			AccessPermissions: duplexPipeAccessPermissions,
			AccessMode:        windows.SET_ACCESS,
			Inheritance:       windows.NO_INHERITANCE,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(creatorOwnerSID),
			},
		},
	}

	sd, err := windows.BuildSecurityDescriptor(nil, nil, explicitAccessPolicies, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("building security descriptor: %w", err)
	}

	// We don't want the security descriptor to include a SACL, but "S:NO_ACCESS_CONTROL" is appended.
	// Remove it so that we don't set a SACL.
	sdStr := strings.ReplaceAll(sd.String(), "S:NO_ACCESS_CONTROL", "")

	cfg := &winio.PipeConfig{
		SecurityDescriptor: sdStr,
		MessageMode:        true,  // Use message mode so that CloseWrite() is supported
		InputBufferSize:    65536, // Use 64KB buffers to improve performance
		OutputBufferSize:   65536,
	}
	pipePath := fmt.Sprintf(`\\.\pipe\%s_%s`, l.pipeNamePrefix, ulid.New())
	return winio.ListenPipe(pipePath, cfg)
}
