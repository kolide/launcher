//go:build windows

package listener

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func setPipePermissions(pipePath string) error {
	adminSid, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return fmt.Errorf("getting admin SID: %w", err)
	}
	creatorOwnerSid, err := windows.CreateWellKnownSid(windows.WinCreatorOwnerSid)
	if err != nil {
		return fmt.Errorf("getting creator/owner SID: %w", err)
	}
	systemSid, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return fmt.Errorf("getting system SID: %w", err)
	}

	// SYSTEM, admin, and creator/owner have full control; standard users are not granted any permissions.
	// We do not inherit permissions from the root directory here.
	explicitAccessPolicies := []windows.EXPLICIT_ACCESS{
		{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.SET_ACCESS,
			Inheritance:       windows.NO_INHERITANCE,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(systemSid),
			},
		},
		{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.SET_ACCESS,
			Inheritance:       windows.NO_INHERITANCE,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(adminSid),
			},
		},
		{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.SET_ACCESS,
			Inheritance:       windows.NO_INHERITANCE,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(creatorOwnerSid),
			},
		},
	}

	// Create a new DACL to overwrite the existing one
	newDacl, err := windows.ACLFromEntries(explicitAccessPolicies, nil)
	if err != nil {
		return fmt.Errorf("generating DACL: %w", err)
	}

	// Apply new DACL
	if err := windows.SetNamedSecurityInfo(
		pipePath,
		windows.SE_FILE_OBJECT,
		// PROTECTED_DACL_SECURITY_INFORMATION here ensures we don't re-inherit the parent permissions
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, newDacl, nil,
	); err != nil {
		return fmt.Errorf("applying new DACL: %w", err)
	}

	return nil
}
