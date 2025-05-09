package windowsupdate

import (
	"fmt"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"github.com/kolide/launcher/pkg/windows/oleconv"
)

// ISearchResult represents the result of a search.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-isearchresult
type ISearchResult struct {
	disp           *ole.IDispatch
	ResultCode     int32 // enum https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-operationresultcode
	RootCategories []*ICategory
	Updates        []*IUpdate
	Warnings       []*IUpdateException
}

func toISearchResult(searchResultDisp *ole.IDispatch) (*ISearchResult, error) {
	var err error
	iSearchResult := &ISearchResult{
		disp: searchResultDisp,
	}

	if iSearchResult.ResultCode, err = oleconv.ToInt32Err(oleutil.GetProperty(searchResultDisp, "ResultCode")); err != nil {
		return nil, fmt.Errorf("getting property ResultCode as int32: %w", err)
	}

	rootCategoriesDisp, err := oleconv.ToIDispatchErr(oleutil.GetProperty(searchResultDisp, "RootCategories"))
	if err != nil {
		return nil, fmt.Errorf("getting property RootCategories as IDispatch: %w", err)
	}
	if rootCategoriesDisp != nil {
		if iSearchResult.RootCategories, err = toICategories(rootCategoriesDisp); err != nil {
			return nil, fmt.Errorf("converting RootCategories IDispatch to ICategories: %w", err)
		}
	}

	// Updates is a IUpdateCollection, and we want the full details. So cast it ia toIUpdates
	updatesDisp, err := oleconv.ToIDispatchErr(oleutil.GetProperty(searchResultDisp, "Updates"))
	if err != nil {
		return nil, fmt.Errorf("getting property Updates as IDispatch: %w", err)
	}
	if updatesDisp != nil {
		if iSearchResult.Updates, err = toIUpdates(updatesDisp); err != nil {
			return nil, fmt.Errorf("converting Updates IDispatch to IUpdates: %w", err)
		}
	}

	warningsDisp, err := oleconv.ToIDispatchErr(oleutil.GetProperty(searchResultDisp, "Warnings"))
	if err != nil {
		return nil, fmt.Errorf("getting property Warnings as IDispatch: %w", err)
	}
	if warningsDisp != nil {
		if iSearchResult.Warnings, err = toIUpdateExceptions(warningsDisp); err != nil {
			return nil, fmt.Errorf("converting Warnings IDispatch to IUpdateExceptions: %w", err)
		}
	}

	return iSearchResult, nil
}
