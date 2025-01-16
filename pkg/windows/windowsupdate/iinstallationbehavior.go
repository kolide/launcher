package windowsupdate

import (
	"context"

	"github.com/go-ole/go-ole"
	"github.com/kolide/launcher/pkg/traces"
)

// IInstallationBehavior represents the installation and uninstallation options of an update.
// https://docs.microsoft.com/zh-cn/windows/win32/api/wuapi/nn-wuapi-iinstallationbehavior
type IInstallationBehavior struct {
	disp                        *ole.IDispatch //nolint:unused
	CanRequestUserInput         bool
	Impact                      int32 // enum https://docs.microsoft.com/zh-cn/windows/win32/api/wuapi/ne-wuapi-installationimpact
	RebootBehavior              int32 // enum https://docs.microsoft.com/zh-cn/windows/win32/api/wuapi/ne-wuapi-installationrebootbehavior
	RequiresNetworkConnectivity bool
}

func toIInstallationBehavior(ctx context.Context, installationBehaviorDisp *ole.IDispatch) (*IInstallationBehavior, error) {
	_, span := traces.StartSpan(ctx)
	defer span.End()

	// TODO
	return nil, nil
}
