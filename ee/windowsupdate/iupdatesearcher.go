package windowsupdate

import (
	"fmt"

	"github.com/go-ole/go-ole"
)

// IUpdateSearcher searches for updates on a server.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdatesearcher
type IUpdateSearcher struct {
	disp                                *ole.IDispatch
	CanAutomaticallyUpgradeService      bool
	ClientApplicationID                 string
	IncludePotentiallySupersededUpdates bool
	Online                              bool
	ServerSelection                     int32
	ServiceID                           string
}

func toIUpdateSearcher(updateSearcherDisp *ole.IDispatch) (*IUpdateSearcher, error) {
	var err error
	iUpdateSearcher := &IUpdateSearcher{
		disp: updateSearcherDisp,
	}

	if iUpdateSearcher.CanAutomaticallyUpgradeService, err = getPropertyBool(updateSearcherDisp, "CanAutomaticallyUpgradeService"); err != nil {
		return nil, fmt.Errorf("CanAutomaticallyUpgradeService: %w", err)
	}

	if iUpdateSearcher.ClientApplicationID, err = getPropertyString(updateSearcherDisp, "ClientApplicationID"); err != nil {
		return nil, fmt.Errorf("ClientApplicationID: %w", err)
	}

	if iUpdateSearcher.IncludePotentiallySupersededUpdates, err = getPropertyBool(updateSearcherDisp, "IncludePotentiallySupersededUpdates"); err != nil {
		return nil, fmt.Errorf("IncludePotentiallySupersededUpdates: %w", err)
	}

	if iUpdateSearcher.Online, err = getPropertyBool(updateSearcherDisp, "Online"); err != nil {
		return nil, fmt.Errorf("Online: %w", err)
	}

	if iUpdateSearcher.ServerSelection, err = getPropertyInt32(updateSearcherDisp, "ServerSelection"); err != nil {
		return nil, fmt.Errorf("ServerSelection: %w", err)
	}

	if iUpdateSearcher.ServiceID, err = getPropertyString(updateSearcherDisp, "ServiceID"); err != nil {
		return nil, fmt.Errorf("ServiceID: %w", err)
	}

	return iUpdateSearcher, nil
}

// Search performs a synchronous search for updates. The search uses the search options that are currently configured.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdatesearcher-search
func (iUpdateSearcher *IUpdateSearcher) Search(criteria string) (*ISearchResult, error) {
	searchResultDisp, err := callMethodDispatch(iUpdateSearcher.disp, "Search", criteria)
	if err != nil {
		return nil, fmt.Errorf("Search: %w", err)
	}
	return toISearchResult(searchResultDisp)
}

// QueryHistory synchronously queries the computer for the history of the update events.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdatesearcher-queryhistory
func (iUpdateSearcher *IUpdateSearcher) QueryHistory(startIndex int32, count int32) ([]*IUpdateHistoryEntry, error) {
	updateHistoryEntriesDisp, err := callMethodDispatch(iUpdateSearcher.disp, "QueryHistory", startIndex, count)
	if err != nil {
		return nil, fmt.Errorf("QueryHistory: %w", err)
	}
	defer updateHistoryEntriesDisp.Release()
	return toIUpdateHistoryEntries(updateHistoryEntriesDisp)
}

// GetTotalHistoryCount returns the number of update events on the computer.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdatesearcher-gettotalhistorycount
func (iUpdateSearcher *IUpdateSearcher) GetTotalHistoryCount() (int32, error) {
	return callMethodInt32(iUpdateSearcher.disp, "GetTotalHistoryCount")
}

// QueryHistoryAll synchronously queries the computer for the history of all update events.
func (iUpdateSearcher *IUpdateSearcher) QueryHistoryAll() ([]*IUpdateHistoryEntry, error) {
	count, err := iUpdateSearcher.GetTotalHistoryCount()
	if err != nil {
		return nil, fmt.Errorf("getting total history count: %w", err)
	}
	return iUpdateSearcher.QueryHistory(0, count)
}
