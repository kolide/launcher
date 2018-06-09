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
	int *doesAutomaticCriticalUpdateInstall,
	int *lastCheckTimestamp
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
	NSDate * lastCheckSuccessfulDate = (NSDate *)[manager lastCheckSuccessfulDate];
	*lastCheckTimestamp = [lastCheckSuccessfulDate timeIntervalSince1970];
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
		table.IntegerColumn("download"),
		table.IntegerColumn("app_updates"),
		table.IntegerColumn("os_updates"),
		table.IntegerColumn("critical_updates"),
		table.IntegerColumn("last_successful_check_timestamp"),
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
		lastCheckTimestamp                    = C.int(0)
	)
	C.softwareUpdate(
		&isAutomaticallyCheckForUpdatesManaged,
		&isAutomaticallyCheckForUpdatesEnabled,
		&doesBackgroundDownload,
		&doesAppStoreAutoUpdates,
		&doesOSXAutoUpdates,
		&doesAutomaticCriticalUpdateInstall,
		&lastCheckTimestamp,
	)

	resp := []map[string]string{
		map[string]string{
			"autoupdate_managed":              fmt.Sprintf("%d", isAutomaticallyCheckForUpdatesManaged),
			"autoupdate_enabled":              fmt.Sprintf("%d", isAutomaticallyCheckForUpdatesEnabled),
			"download":                        fmt.Sprintf("%d", doesBackgroundDownload),
			"app_updates":                     fmt.Sprintf("%d", doesAppStoreAutoUpdates),
			"os_updates":                      fmt.Sprintf("%d", doesOSXAutoUpdates),
			"critical_updates":                fmt.Sprintf("%d", doesAutomaticCriticalUpdateInstall),
			"last_successful_check_timestamp": fmt.Sprintf("%d", lastCheckTimestamp),
		},
	}
	return resp, nil
}
