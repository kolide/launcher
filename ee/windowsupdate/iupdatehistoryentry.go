package windowsupdate

import (
	"fmt"
	"time"

	"github.com/go-ole/go-ole"
)

// IUpdateHistoryEntry represents the recorded history of an update.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdatehistoryentry
type IUpdateHistoryEntry struct {
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
	count, err := getPropertyInt32(updateHistoryEntriesDisp, "Count")
	if err != nil {
		return nil, fmt.Errorf("Count: %w", err)
	}

	updateHistoryEntries := make([]*IUpdateHistoryEntry, count)
	for i := 0; i < int(count); i++ {
		entryDisp, err := getPropertyDispatch(updateHistoryEntriesDisp, "Item", i)
		if err != nil {
			return nil, fmt.Errorf("Item[%d/%d]: %w", i, count, err)
		}

		entry, err := toIUpdateHistoryEntry(entryDisp)
		if err != nil {
			return nil, fmt.Errorf("converting Item[%d/%d]: %w", i, count, err)
		}

		updateHistoryEntries[i] = entry
	}
	return updateHistoryEntries, nil
}

func toIUpdateHistoryEntry(entryDisp *ole.IDispatch) (*IUpdateHistoryEntry, error) {
	defer entryDisp.Release()

	var err error
	entry := &IUpdateHistoryEntry{}

	if entry.ClientApplicationID, err = getPropertyString(entryDisp, "ClientApplicationID"); err != nil {
		return nil, fmt.Errorf("ClientApplicationID: %w", err)
	}

	if entry.Date, err = getPropertyTime(entryDisp, "Date"); err != nil {
		return nil, fmt.Errorf("Date: %w", err)
	}

	if entry.Description, err = getPropertyString(entryDisp, "Description"); err != nil {
		return nil, fmt.Errorf("Description: %w", err)
	}

	if entry.HResult, err = getPropertyInt32(entryDisp, "HResult"); err != nil {
		return nil, fmt.Errorf("HResult: %w", err)
	}

	if entry.Operation, err = getPropertyInt32(entryDisp, "Operation"); err != nil {
		return nil, fmt.Errorf("Operation: %w", err)
	}

	if entry.ResultCode, err = getPropertyInt32(entryDisp, "ResultCode"); err != nil {
		return nil, fmt.Errorf("ResultCode: %w", err)
	}

	if entry.ServerSelection, err = getPropertyInt32(entryDisp, "ServerSelection"); err != nil {
		return nil, fmt.Errorf("ServerSelection: %w", err)
	}

	if entry.ServiceID, err = getPropertyString(entryDisp, "ServiceID"); err != nil {
		return nil, fmt.Errorf("ServiceID: %w", err)
	}

	if entry.SupportUrl, err = getPropertyString(entryDisp, "SupportUrl"); err != nil {
		return nil, fmt.Errorf("SupportUrl: %w", err)
	}

	if entry.Title, err = getPropertyString(entryDisp, "Title"); err != nil {
		return nil, fmt.Errorf("Title: %w", err)
	}

	if entry.UninstallationNotes, err = getPropertyString(entryDisp, "UninstallationNotes"); err != nil {
		return nil, fmt.Errorf("UninstallationNotes: %w", err)
	}

	// UninstallationSteps is a string collection
	if uninstallStepsDisp, err := getPropertyDispatch(entryDisp, "UninstallationSteps"); err != nil {
		return nil, fmt.Errorf("UninstallationSteps: %w", err)
	} else if uninstallStepsDisp != nil {
		defer uninstallStepsDisp.Release()
		if entry.UninstallationSteps, err = iStringCollectionToStringArray(uninstallStepsDisp); err != nil {
			return nil, fmt.Errorf("converting UninstallationSteps: %w", err)
		}
	}

	if entry.UnmappedResultCode, err = getPropertyInt32(entryDisp, "UnmappedResultCode"); err != nil {
		return nil, fmt.Errorf("UnmappedResultCode: %w", err)
	}

	if identityDisp, err := getPropertyDispatch(entryDisp, "UpdateIdentity"); err != nil {
		return nil, fmt.Errorf("UpdateIdentity: %w", err)
	} else if identityDisp != nil {
		// toIUpdateIdentity calls Release() on identityDisp internally
		if entry.UpdateIdentity, err = toIUpdateIdentity(identityDisp); err != nil {
			return nil, fmt.Errorf("converting UpdateIdentity: %w", err)
		}
	}

	return entry, nil
}

