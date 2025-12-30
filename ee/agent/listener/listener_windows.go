//go:build windows

package listener

import (
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
	"github.com/kolide/kit/ulid"
	"golang.org/x/sys/windows"
)

const (
	FILE_CREATE_PIPE_INSTANCE = 0x00000004
	// duplexPipeAccessPermissions includes read, write, and sync permissions for sending data,
	// FILE_CREATE_PIPE_INSTANCE to create the named pipe, and ACCESS_SYSTEM_SECURITY to
	// set the SACL on the named pipe.
	// See: https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipe-security-and-access-rights
	duplexPipeAccessPermissions = windows.GENERIC_READ | windows.GENERIC_WRITE | windows.SYNCHRONIZE | FILE_CREATE_PIPE_INSTANCE | windows.ACCESS_SYSTEM_SECURITY
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

	cfg := &winio.PipeConfig{
		SecurityDescriptor: sd.String(),
		MessageMode:        true,  // Use message mode so that CloseWrite() is supported
		InputBufferSize:    65536, // Use 64KB buffers to improve performance
		OutputBufferSize:   65536,
	}
	pipePath := fmt.Sprintf(`\\.\pipe\%s_%s`, l.pipeNamePrefix, ulid.New())
	return winio.ListenPipe(pipePath, cfg)
}
