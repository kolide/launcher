package windowsupdate

import (
	"fmt"

	"github.com/go-ole/go-ole"
)

// ISearchResult represents the result of a search.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-isearchresult
type ISearchResult struct {
	ResultCode     int32 // enum https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-operationresultcode
	RootCategories []*ICategory
	Updates        []*IUpdate
	Warnings       []*IUpdateException
}

func toISearchResult(searchResultDisp *ole.IDispatch) (*ISearchResult, error) {
	defer searchResultDisp.Release()

	var err error
	iSearchResult := &ISearchResult{}

	if iSearchResult.ResultCode, err = getPropertyInt32(searchResultDisp, "ResultCode"); err != nil {
		return nil, fmt.Errorf("ResultCode: %w", err)
	}

	rootCategoriesDisp, err := getPropertyDispatch(searchResultDisp, "RootCategories")
	if err != nil {
		return nil, fmt.Errorf("RootCategories: %w", err)
	}
	if rootCategoriesDisp != nil {
		defer rootCategoriesDisp.Release()
		if iSearchResult.RootCategories, err = toICategories(rootCategoriesDisp); err != nil {
			return nil, fmt.Errorf("converting RootCategories: %w", err)
		}
	}

	updatesDisp, err := getPropertyDispatch(searchResultDisp, "Updates")
	if err != nil {
		return nil, fmt.Errorf("Updates: %w", err)
	}
	if updatesDisp != nil {
		defer updatesDisp.Release()
		if iSearchResult.Updates, err = toIUpdates(updatesDisp); err != nil {
			return nil, fmt.Errorf("converting Updates: %w", err)
		}
	}

	warningsDisp, err := getPropertyDispatch(searchResultDisp, "Warnings")
	if err != nil {
		return nil, fmt.Errorf("Warnings: %w", err)
	}
	if warningsDisp != nil {
		defer warningsDisp.Release()
		if iSearchResult.Warnings, err = toIUpdateExceptions(warningsDisp); err != nil {
			return nil, fmt.Errorf("converting Warnings: %w", err)
		}
	}

	return iSearchResult, nil
}
