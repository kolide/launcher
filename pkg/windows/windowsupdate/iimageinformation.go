package windowsupdate

import (
	"context"

	"github.com/go-ole/go-ole"
	"github.com/kolide/launcher/pkg/traces"
)

// IImageInformation contains information about a localized image that is associated with an update or a category.
// https://docs.microsoft.com/zh-cn/windows/win32/api/wuapi/nn-wuapi-iimageinformation
type IImageInformation struct {
	disp    *ole.IDispatch //nolint:unused
	AltText string
	Height  int64
	Source  string
	Width   int64
}

func toIImageInformation(ctx context.Context, imageInformationDisp *ole.IDispatch) (*IImageInformation, error) {
	_, span := traces.StartSpan(ctx)
	defer span.End()

	// TODO
	return nil, nil
}
