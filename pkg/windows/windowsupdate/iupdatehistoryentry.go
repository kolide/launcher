package windowsupdate

import (
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"github.com/kolide/launcher/pkg/windows/oleconv"
	"github.com/pkg/errors"
)

// IUpdateHistoryEntry represents the recorded history of an update.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdatehistoryentry
type IUpdateHistoryEntry struct {
	disp                *ole.IDispatch
	ClientApplicationID string
	Date                *time.Time
	Description         string
	HResult             int32
	Operation           int32 // enum https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-updateoperation
	ResultCode          int32 // enum https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-operationresultcode
	ServerSelection     int32 // enum
	ServiceID           string
	SupportUrl          string
	Title               string
	UninstallationNotes string
	UninstallationSteps []string
	UnmappedResultCode  int32
	UpdateIdentity      *IUpdateIdentity
}

func toIUpdateHistoryEntries(updateHistoryEntriesDisp *ole.IDispatch) ([]*IUpdateHistoryEntry, error) {
	count, err := oleconv.ToInt32Err(oleutil.GetProperty(updateHistoryEntriesDisp, "Count"))
	if err != nil {
		return nil, errors.Wrap(err, "Count")
	}

	updateHistoryEntries := make([]*IUpdateHistoryEntry, count)
	for i := 0; i < int(count); i++ {
		updateHistoryEntryDisp, err := oleconv.ToIDispatchErr(oleutil.GetProperty(updateHistoryEntriesDisp, "Item", i))
		if err != nil {
			return nil, errors.Wrapf(err, "item %d", i)
		}

		updateHistoryEntry, err := toIUpdateHistoryEntry(updateHistoryEntryDisp)
		if err != nil {
			return nil, errors.Wrap(err, "toIUpdateHistoryEntry")
		}

		updateHistoryEntries[i] = updateHistoryEntry
	}
	return updateHistoryEntries, nil
}

func toIUpdateHistoryEntry(updateHistoryEntryDisp *ole.IDispatch) (*IUpdateHistoryEntry, error) {
	var err error
	iUpdateHistoryEntry := &IUpdateHistoryEntry{
		disp: updateHistoryEntryDisp,
	}

	if iUpdateHistoryEntry.ClientApplicationID, err = oleconv.ToStringErr(oleutil.GetProperty(updateHistoryEntryDisp, "ClientApplicationID")); err != nil {
		return nil, errors.Wrap(err, "ClientApplicationID")
	}

	if iUpdateHistoryEntry.Date, err = oleconv.ToTimeErr(oleutil.GetProperty(updateHistoryEntryDisp, "Date")); err != nil {
		return nil, errors.Wrap(err, "Date")
	}

	if iUpdateHistoryEntry.Description, err = oleconv.ToStringErr(oleutil.GetProperty(updateHistoryEntryDisp, "Description")); err != nil {
		return nil, errors.Wrap(err, "Description")
	}

	if iUpdateHistoryEntry.HResult, err = oleconv.ToInt32Err(oleutil.GetProperty(updateHistoryEntryDisp, "HResult")); err != nil {
		return nil, errors.Wrap(err, "HResult")
	}

	if iUpdateHistoryEntry.Operation, err = oleconv.ToInt32Err(oleutil.GetProperty(updateHistoryEntryDisp, "Operation")); err != nil {
		return nil, errors.Wrap(err, "Operation")
	}

	if iUpdateHistoryEntry.ResultCode, err = oleconv.ToInt32Err(oleutil.GetProperty(updateHistoryEntryDisp, "ResultCode")); err != nil {
		return nil, errors.Wrap(err, "ResultCode")
	}

	if iUpdateHistoryEntry.ServerSelection, err = oleconv.ToInt32Err(oleutil.GetProperty(updateHistoryEntryDisp, "ServerSelection")); err != nil {
		return nil, errors.Wrap(err, "ServerSelection")
	}

	if iUpdateHistoryEntry.ServiceID, err = oleconv.ToStringErr(oleutil.GetProperty(updateHistoryEntryDisp, "ServiceID")); err != nil {
		return nil, errors.Wrap(err, "ServiceID")
	}

	if iUpdateHistoryEntry.SupportUrl, err = oleconv.ToStringErr(oleutil.GetProperty(updateHistoryEntryDisp, "SupportUrl")); err != nil {
		return nil, errors.Wrap(err, "SupportUrl")
	}

	if iUpdateHistoryEntry.Title, err = oleconv.ToStringErr(oleutil.GetProperty(updateHistoryEntryDisp, "Title")); err != nil {
		return nil, errors.Wrap(err, "Title")
	}

	if iUpdateHistoryEntry.UninstallationNotes, err = oleconv.ToStringErr(oleutil.GetProperty(updateHistoryEntryDisp, "UninstallationNotes")); err != nil {
		return nil, errors.Wrap(err, "UninstallationNotes")
	}

	if iUpdateHistoryEntry.UninstallationSteps, err = oleconv.ToStringSliceErr(oleutil.GetProperty(updateHistoryEntryDisp, "UninstallationSteps")); err != nil {
		return nil, errors.Wrap(err, "UninstallationSteps")
	}

	if iUpdateHistoryEntry.UnmappedResultCode, err = oleconv.ToInt32Err(oleutil.GetProperty(updateHistoryEntryDisp, "UnmappedResultCode")); err != nil {
		return nil, errors.Wrap(err, "UnmappedResultCode")
	}

	updateIdentityDisp, err := oleconv.ToIDispatchErr(oleutil.GetProperty(updateHistoryEntryDisp, "UpdateIdentity"))
	if err != nil {
		return nil, errors.Wrap(err, "UpdateIdentity")
	}
	if updateIdentityDisp != nil {
		if iUpdateHistoryEntry.UpdateIdentity, err = toIUpdateIdentity(updateIdentityDisp); err != nil {
			return nil, errors.Wrap(err, "toIUpdateIdentity")
		}
	}

	return iUpdateHistoryEntry, nil
}
