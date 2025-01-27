package windowsupdate

import (
	"context"

	"github.com/go-ole/go-ole"
	"github.com/kolide/launcher/pkg/traces"
)

// IUpdateException represents info about the aspects of search results returned in the ISearchResult object that were incomplete. For more info, see Remarks.
// https://docs.microsoft.com/zh-cn/windows/win32/api/wuapi/nn-wuapi-iupdateexception
type IUpdateException struct {
	disp    *ole.IDispatch //nolint:unused
	Context int32          // enum https://docs.microsoft.com/zh-cn/windows/win32/api/wuapi/ne-wuapi-updateexceptioncontext
	HResult int64
	Message string
}

func toIUpdateExceptions(ctx context.Context, updateExceptionsDisp *ole.IDispatch) ([]*IUpdateException, error) {
	_, span := traces.StartSpan(ctx)
	defer span.End()

	// TODO
	return nil, nil
}
