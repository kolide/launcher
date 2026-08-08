package windowsupdate

import (
	"fmt"

	"github.com/go-ole/go-ole"
)

// IUpdateIdentity represents the unique identifier of an update.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdateidentity
type IUpdateIdentity struct {
	RevisionNumber int32
	UpdateID       string
}

func toIUpdateIdentity(updateIdentityDisp *ole.IDispatch) (*IUpdateIdentity, error) {
	defer updateIdentityDisp.Release()

	var err error
	iUpdateIdentity := &IUpdateIdentity{}

	if iUpdateIdentity.RevisionNumber, err = getPropertyInt32(updateIdentityDisp, "RevisionNumber"); err != nil {
		return nil, fmt.Errorf("RevisionNumber: %w", err)
	}

	if iUpdateIdentity.UpdateID, err = getPropertyString(updateIdentityDisp, "UpdateID"); err != nil {
		return nil, fmt.Errorf("UpdateID: %w", err)
	}

	return iUpdateIdentity, nil
}
