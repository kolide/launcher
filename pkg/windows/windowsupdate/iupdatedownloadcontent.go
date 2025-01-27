package windowsupdate

import (
	"context"

	"github.com/go-ole/go-ole"
	"github.com/kolide/launcher/pkg/traces"
)

// IUpdateDownloadContent represents the download content of an update.
// https://docs.microsoft.com/zh-cn/windows/win32/api/wuapi/nn-wuapi-iupdatedownloadcontent
type IUpdateDownloadContent struct {
	disp        *ole.IDispatch //nolint:unused
	DownloadUrl string
}

func toIUpdateDownloadContents(ctx context.Context, updateDownloadContentsDisp *ole.IDispatch) ([]*IUpdateDownloadContent, error) {
	_, span := traces.StartSpan(ctx)
	defer span.End()

	// TODO
	return nil, nil
}
