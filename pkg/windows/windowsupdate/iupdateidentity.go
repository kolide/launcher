package windowsupdate

import (
	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"github.com/kolide/launcher/pkg/windows/oleconv"
	"github.com/pkg/errors"
)

// IUpdateIdentity represents the unique identifier of an update.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdateidentity
type IUpdateIdentity struct {
	disp           *ole.IDispatch
	RevisionNumber int32
	UpdateID       string
}

func toIUpdateIdentity(updateIdentityDisp *ole.IDispatch) (*IUpdateIdentity, error) {
	var err error
	iUpdateIdentity := &IUpdateIdentity{
		disp: updateIdentityDisp,
	}

	if iUpdateIdentity.RevisionNumber, err = oleconv.ToInt32Err(oleutil.GetProperty(updateIdentityDisp, "RevisionNumber")); err != nil {
		return nil, errors.Wrap(err, "RevisionNumber")
	}

	if iUpdateIdentity.UpdateID, err = oleconv.ToStringErr(oleutil.GetProperty(updateIdentityDisp, "UpdateID")); err != nil {
		return nil, errors.Wrap(err, "UpdateID")
	}

	return iUpdateIdentity, nil
}
