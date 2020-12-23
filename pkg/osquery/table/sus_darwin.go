package table

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#cgo darwin LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#import <SUSharedPrefs.h>
void softwareUpdate(
	int os_version,
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

	// before 10.13 (build ver 17 (build ver 18) it's called doesMacOSAutoUpdate.
	if (os_version >= 18) {
		val = [manager doesMacOSAutoUpdate];
		if (val) {
			*doesOSXAutoUpdates = 1;
		}
	} else {
		val = [manager doesOSXAutoUpdates];
		if (val) {
			*doesOSXAutoUpdates = 1;
		}
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
	"strconv"

	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

func MacOSUpdate(client *osquery.ExtensionManagerClient) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.IntegerColumn("autoupdate_managed"),
		table.IntegerColumn("autoupdate_enabled"),
		table.IntegerColumn("download"),
		table.IntegerColumn("app_updates"),
		table.IntegerColumn("os_updates"),
		table.IntegerColumn("critical_updates"),
		table.IntegerColumn("last_successful_check_timestamp"),
	}
	tableGen := &osUpdateTable{client: client}
	return table.NewPlugin("kolide_macos_software_update", columns, tableGen.generateMacUpdate)
}

type osUpdateTable struct {
	client                  *osquery.ExtensionManagerClient
	macOSBuildVersionPrefix int
}

func (table *osUpdateTable) generateMacUpdate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	if table.macOSBuildVersionPrefix == 0 {
		buildPrefix, err := macOSBuildVersionPrefix(table.client)
		if err != nil {
			return nil, errors.Wrap(err, "determine macOS build prefix for software update table")
		}
		table.macOSBuildVersionPrefix = buildPrefix
	}
	var (
		version                               = C.int(table.macOSBuildVersionPrefix)
		isAutomaticallyCheckForUpdatesManaged = C.int(0)
		isAutomaticallyCheckForUpdatesEnabled = C.int(0)
		doesBackgroundDownload                = C.int(0)
		doesAppStoreAutoUpdates               = C.int(0)
		doesOSXAutoUpdates                    = C.int(0)
		doesAutomaticCriticalUpdateInstall    = C.int(0)
		lastCheckTimestamp                    = C.int(0)
	)
	C.softwareUpdate(
		version,
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

func macOSBuildVersionPrefix(client *osquery.ExtensionManagerClient) (int, error) {
	query := `SELECT CAST(SUBSTR(build,0,3) AS int) AS build_prefix FROM os_version`
	row, err := client.QueryRow(query)
	if err != nil {
		return 0, errors.Wrap(err, "querying for macOS version")
	}
	buildPrefix, err := strconv.Atoi(row["build_prefix"])
	if err != nil {
		return 0, errors.Wrap(err, "converting build prefix from string to int")
	}
	return buildPrefix, nil
}
