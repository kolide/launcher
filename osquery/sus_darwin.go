package osquery

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#cgo darwin LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#import <SUSharedPrefs.h>
void softwareUpdate(
	int *isAutomaticallyCheckForUpdatesManaged,
	int *isAutomaticallyCheckForUpdatesEnabled,
	int *doesBackgroundDownload,
	int *doesAppStoreAutoUpdates,
	int *doesOSXAutoUpdates,
	int *doesAutomaticCriticalUpdateInstall
) {
	NSBundle *bundle;
	bundle = [NSBundle bundleWithPath:@"/System/Library/PrivateFrameworks/SoftwareUpdate.framework"];
	[bundle load];

	Class SUSharedPrefs = [bundle classNamed:@"SUSharedPrefs"];
	id manager = [SUSharedPrefs sharedPrefManager];

	BOOL val = [manager isAutomaticallyCheckForUpdatesManaged];
	if (val) {
		*isAutomaticallyCheckForUpdatesManaged = 1;
	}

	val = [manager isAutomaticallyCheckForUpdatesEnabled];
	if (val) {
		*isAutomaticallyCheckForUpdatesEnabled = 1;
	}

	val = [manager doesBackgroundDownload];
	if (val) {
		*doesBackgroundDownload = 1;
	}

	val = [manager doesAppStoreAutoUpdates];
	if (val) {
		*doesAppStoreAutoUpdates = 1;
	}

	val = [manager doesOSXAutoUpdates];
	if (val) {
		*doesOSXAutoUpdates = 1;
	}


	val = [manager doesAutomaticCriticalUpdateInstall];
	if (val) {
		*doesAutomaticCriticalUpdateInstall = 1;
	}
	return;
}
*/
import "C"
import (
	"context"
	"fmt"

	"github.com/kolide/osquery-go/plugin/table"
)

func MacUpdate() *table.Plugin {
	columns := []table.ColumnDefinition{
		table.IntegerColumn("autoupdate_managed"),
		table.IntegerColumn("autoupdate_enabled"),
		table.IntegerColumn("background_download"),
		table.IntegerColumn("app_store_apps_autoupdate"),
		table.IntegerColumn("macos_autoupdate"),
		table.IntegerColumn("critical_updates"),
	}
	return table.NewPlugin("kolide_macos_software_update", columns, generateMacUpdate)
}

func generateMacUpdate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var (
		isAutomaticallyCheckForUpdatesManaged = C.int(0)
		isAutomaticallyCheckForUpdatesEnabled = C.int(0)
		doesBackgroundDownload                = C.int(0)
		doesAppStoreAutoUpdates               = C.int(0)
		doesOSXAutoUpdates                    = C.int(0)
		doesAutomaticCriticalUpdateInstall    = C.int(0)
	)
	C.softwareUpdate(
		&isAutomaticallyCheckForUpdatesManaged,
		&isAutomaticallyCheckForUpdatesEnabled,
		&doesBackgroundDownload,
		&doesAppStoreAutoUpdates,
		&doesOSXAutoUpdates,
		&doesAutomaticCriticalUpdateInstall,
	)

	resp := []map[string]string{
		map[string]string{
			"autoupdate_managed":        fmt.Sprintf("%d", isAutomaticallyCheckForUpdatesManaged),
			"autoupdate_enabled":        fmt.Sprintf("%d", isAutomaticallyCheckForUpdatesEnabled),
			"background_download":       fmt.Sprintf("%d", doesBackgroundDownload),
			"app_store_apps_autoupdate": fmt.Sprintf("%d", doesAppStoreAutoUpdates),
			"macos_autoupdate":          fmt.Sprintf("%d", doesOSXAutoUpdates),
			"critical_updates":          fmt.Sprintf("%d", doesAutomaticCriticalUpdateInstall),
		},
	}
	return resp, nil
}
