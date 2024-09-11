//go:build darwin
// +build darwin

package macos_software_update

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#cgo darwin LDFLAGS: -framework Cocoa
#include "sus.h"
*/
import (
	"C"
)
import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/osquery/osquery-go/plugin/table"
	"golang.org/x/sys/unix"
)

func MacOSUpdate() *table.Plugin {
	columns := []table.ColumnDefinition{
		table.IntegerColumn("autoupdate_managed"),
		table.IntegerColumn("autoupdate_enabled"),
		table.IntegerColumn("download"),
		table.IntegerColumn("app_updates"),
		table.IntegerColumn("os_updates"),
		table.IntegerColumn("critical_updates"),
		table.IntegerColumn("last_successful_check_timestamp"),
	}
	tableGen := &osUpdateTable{}
	return table.NewPlugin("kolide_macos_software_update", columns, tableGen.generateMacUpdate)
}

type osUpdateTable struct {
	macOSBuildVersionPrefix int
}

func (table *osUpdateTable) generateMacUpdate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	if table.macOSBuildVersionPrefix == 0 {
		buildPrefix, err := macOSBuildVersionPrefix()
		if err != nil {
			return nil, fmt.Errorf("determine macOS build prefix for software update table: %w", err)
		}
		table.macOSBuildVersionPrefix = buildPrefix
	}
	var (
		version                               = C.int(table.macOSBuildVersionPrefix)
		isMacOSAutoUpdateManaged              = C.int(0)
		isAutomaticallyCheckForUpdatesEnabled = C.int(0)
		doesBackgroundDownload                = C.int(0)
		doesAppStoreAutoUpdates               = C.int(0)
		doesOSXAutoUpdates                    = C.int(0)
		doesAutomaticCriticalUpdateInstall    = C.int(0)
		lastCheckTimestamp                    = C.int(0)
	)
	C.getSoftwareUpdateConfiguration(
		version,
		&isMacOSAutoUpdateManaged,
		&isAutomaticallyCheckForUpdatesEnabled,
		&doesBackgroundDownload,
		&doesAppStoreAutoUpdates,
		&doesOSXAutoUpdates,
		&doesAutomaticCriticalUpdateInstall,
		&lastCheckTimestamp,
	)

	resp := []map[string]string{
		{
			"autoupdate_managed":              fmt.Sprintf("%d", isMacOSAutoUpdateManaged),
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

func macOSBuildVersionPrefix() (int, error) {
	version, err := unix.Sysctl("kern.osrelease")
	if err != nil {
		return 0, err
	}

	parts := strings.Split(version, ".")
	if len(parts) < 1 {
		return 0, fmt.Errorf("failed to parse build train prefix from sysctl call for kern.osrelease")
	}

	buildPrefix, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("converting build prefix from string to int: %w", err)
	}

	return buildPrefix, nil
}
