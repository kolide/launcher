package windowsupdate

import (
	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"github.com/kolide/launcher/pkg/windows/oleconv"
	"github.com/pkg/errors"
)

// ISearchResult represents the result of a search.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-isearchresult
type ISearchResult struct {
	disp           *ole.IDispatch
	ResultCode     int32 // enum https://docs.microsoft.com/zh-cn/windows/win32/api/wuapi/ne-wuapi-operationresultcode
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
		return nil, errors.Wrap(err, "ResultCode")
	}

	rootCategoriesDisp, err := oleconv.ToIDispatchErr(oleutil.GetProperty(searchResultDisp, "RootCategories"))
	if err != nil {
		return nil, errors.Wrap(err, "RootCategories")
	}
	if rootCategoriesDisp != nil {
		if iSearchResult.RootCategories, err = toICategories(rootCategoriesDisp); err != nil {
			return nil, errors.Wrap(err, "toICategories")
		}
	}

	updatesDisp, err := oleconv.ToIDispatchErr(oleutil.GetProperty(searchResultDisp, "Updates"))
	if err != nil {
		return nil, errors.Wrap(err, "Updates")
	}
	if updatesDisp != nil {
		if iSearchResult.Updates, err = toIUpdates(updatesDisp); err != nil {
			return nil, errors.Wrap(err, "toIUpdates")
		}
	}

	warningsDisp, err := oleconv.ToIDispatchErr(oleutil.GetProperty(searchResultDisp, "Warnings"))
	if err != nil {
		return nil, errors.Wrap(err, "Warnings")
	}
	if warningsDisp != nil {
		if iSearchResult.Warnings, err = toIUpdateExceptions(warningsDisp); err != nil {
			return nil, errors.Wrap(err, "toIUpdateExceptions")
		}
	}

	return iSearchResult, nil
}
