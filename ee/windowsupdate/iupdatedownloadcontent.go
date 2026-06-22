package windowsupdate

import (
	"github.com/go-ole/go-ole"
)

// IUpdateDownloadContent represents the download content of an update.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdatedownloadcontent
type IUpdateDownloadContent struct {
	DownloadUrl string
}

func toIUpdateDownloadContents(updateDownloadContentsDisp *ole.IDispatch) ([]*IUpdateDownloadContent, error) {
	// TODO: implement property extraction
	return nil, nil
}
