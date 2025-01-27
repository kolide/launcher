package windowsupdate

import (
	"context"
	"fmt"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/kolide/launcher/pkg/windows/oleconv"
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

func toIUpdateSearcher(ctx context.Context, updateSearcherDisp *ole.IDispatch) (*IUpdateSearcher, error) {
	_, span := traces.StartSpan(ctx)
	defer span.End()

	var err error
	iUpdateSearcher := &IUpdateSearcher{
		disp: updateSearcherDisp,
	}

	if iUpdateSearcher.CanAutomaticallyUpgradeService, err = oleconv.ToBoolErr(oleutil.GetProperty(updateSearcherDisp, "CanAutomaticallyUpgradeService")); err != nil {
		return nil, fmt.Errorf("CanAutomaticallyUpgradeService: %w", err)
	}

	if iUpdateSearcher.ClientApplicationID, err = oleconv.ToStringErr(oleutil.GetProperty(updateSearcherDisp, "ClientApplicationID")); err != nil {
		return nil, fmt.Errorf("ClientApplicationID: %w", err)
	}

	if iUpdateSearcher.IncludePotentiallySupersededUpdates, err = oleconv.ToBoolErr(oleutil.GetProperty(updateSearcherDisp, "IncludePotentiallySupersededUpdates")); err != nil {
		return nil, fmt.Errorf("IncludePotentiallySupersededUpdates: %w", err)
	}

	if iUpdateSearcher.Online, err = oleconv.ToBoolErr(oleutil.GetProperty(updateSearcherDisp, "Online")); err != nil {
		return nil, fmt.Errorf("Online: %w", err)
	}

	if iUpdateSearcher.ServerSelection, err = oleconv.ToInt32Err(oleutil.GetProperty(updateSearcherDisp, "ServerSelection")); err != nil {
		return nil, fmt.Errorf("ServerSelection: %w", err)
	}

	if iUpdateSearcher.ServiceID, err = oleconv.ToStringErr(oleutil.GetProperty(updateSearcherDisp, "ServiceID")); err != nil {
		return nil, fmt.Errorf("ServiceID: %w", err)
	}

	return iUpdateSearcher, nil
}

// Search performs a synchronous search for updates. The search uses the search options that are currently configured.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdatesearcher-search
func (iUpdateSearcher *IUpdateSearcher) Search(ctx context.Context, criteria string) (*ISearchResult, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	searchResultDisp, err := oleconv.ToIDispatchErr(oleutil.CallMethod(iUpdateSearcher.disp, "Search", criteria))
	if err != nil {
		return nil, err
	}
	return toISearchResult(ctx, searchResultDisp)
}

// QueryHistory synchronously queries the computer for the history of the update events.
// https://docs.microsoft.com/zh-cn/windows/win32/api/wuapi/nf-wuapi-iupdatesearcher-queryhistory
func (iUpdateSearcher *IUpdateSearcher) QueryHistory(ctx context.Context, startIndex int32, count int32) ([]*IUpdateHistoryEntry, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	updateHistoryEntriesDisp, err := oleconv.ToIDispatchErr(oleutil.CallMethod(iUpdateSearcher.disp, "QueryHistory", startIndex, count))
	if err != nil {
		return nil, err
	}
	return toIUpdateHistoryEntries(ctx, updateHistoryEntriesDisp)
}

// GetTotalHistoryCount returns the number of update events on the computer.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdatesearcher-gettotalhistorycount
func (iUpdateSearcher *IUpdateSearcher) GetTotalHistoryCount(ctx context.Context) (int32, error) {
	_, span := traces.StartSpan(ctx)
	defer span.End()

	return oleconv.ToInt32Err(oleutil.CallMethod(iUpdateSearcher.disp, "GetTotalHistoryCount"))
}

// QueryHistoryAll synchronously queries the computer for the history of all update events.
func (iUpdateSearcher *IUpdateSearcher) QueryHistoryAll(ctx context.Context) ([]*IUpdateHistoryEntry, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	count, err := iUpdateSearcher.GetTotalHistoryCount(ctx)
	if err != nil {
		return nil, err
	}
	return iUpdateSearcher.QueryHistory(ctx, 0, count)
}
