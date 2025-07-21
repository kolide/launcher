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
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
	"golang.org/x/sys/unix"
)

func MacOSUpdate(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.IntegerColumn("autoupdate_managed"),
		table.IntegerColumn("autoupdate_enabled"),
		table.IntegerColumn("download_managed"),
		table.IntegerColumn("download"),
		table.IntegerColumn("app_updates"),
		table.IntegerColumn("os_updates_managed"),
		table.IntegerColumn("os_updates"),
		table.IntegerColumn("config_data_critical_updates_managed"),
		table.IntegerColumn("config_data_updates"),
		table.IntegerColumn("critical_updates"),
		table.IntegerColumn("last_successful_check_timestamp"),
	}
	tableGen := &osUpdateTable{}
	return tablewrapper.New(flags, slogger, "kolide_macos_software_update", columns, tableGen.generateMacUpdate)
}

type osUpdateTable struct {
	macOSBuildVersionPrefix int
}

func (table *osUpdateTable) generateMacUpdate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	_, span := observability.StartSpan(ctx, "table_name", "kolide_macos_software_update")
	defer span.End()

	if table.macOSBuildVersionPrefix == 0 {
		buildPrefix, err := macOSBuildVersionPrefix()
		if err != nil {
			return nil, fmt.Errorf("determine macOS build prefix for software update table: %w", err)
		}
		table.macOSBuildVersionPrefix = buildPrefix
	}
	var (
		version                                           = C.int(table.macOSBuildVersionPrefix)
		isAutomaticallyCheckForUpdatesManaged             = C.int(0)
		isAutomaticallyCheckForUpdatesEnabled             = C.int(0)
		isdoBackgroundDownloadManaged                     = C.int(0)
		doesBackgroundDownload                            = C.int(0)
		doesAppStoreAutoUpdates                           = C.int(0)
		doesOSXAutoUpdatesManaged                         = C.int(0)
		doesOSXAutoUpdates                                = C.int(0)
		isAutomaticConfigDataCriticalUpdateInstallManaged = C.int(0)
		doesAutomaticConfigDataInstall                    = C.int(0)
		doesAutomaticCriticalUpdateInstall                = C.int(0)
		lastCheckTimestamp                                = C.int(0)
	)
	C.getSoftwareUpdateConfiguration(
		version,
		&isAutomaticallyCheckForUpdatesManaged,
		&isAutomaticallyCheckForUpdatesEnabled,
		&isdoBackgroundDownloadManaged,
		&doesBackgroundDownload,
		&doesAppStoreAutoUpdates,
		&doesOSXAutoUpdatesManaged,
		&doesOSXAutoUpdates,
		&isAutomaticConfigDataCriticalUpdateInstallManaged,
		&doesAutomaticConfigDataInstall,
		&doesAutomaticCriticalUpdateInstall,
		&lastCheckTimestamp,
	)

	resp := []map[string]string{
		{
			"autoupdate_managed":                   fmt.Sprintf("%d", isAutomaticallyCheckForUpdatesManaged),
			"autoupdate_enabled":                   fmt.Sprintf("%d", isAutomaticallyCheckForUpdatesEnabled),
			"download_managed":                     fmt.Sprintf("%d", isdoBackgroundDownloadManaged),
			"download":                             fmt.Sprintf("%d", doesBackgroundDownload),
			"app_updates":                          fmt.Sprintf("%d", doesAppStoreAutoUpdates),
			"os_updates_managed":                   fmt.Sprintf("%d", doesOSXAutoUpdatesManaged),
			"os_updates":                           fmt.Sprintf("%d", doesOSXAutoUpdates),
			"config_data_critical_updates_managed": fmt.Sprintf("%d", isAutomaticConfigDataCriticalUpdateInstallManaged),
			"config_data_updates":                  fmt.Sprintf("%d", doesAutomaticConfigDataInstall),
			"critical_updates":                     fmt.Sprintf("%d", doesAutomaticCriticalUpdateInstall),
			"last_successful_check_timestamp":      fmt.Sprintf("%d", lastCheckTimestamp),
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
		return 0, errors.New("failed to parse build train prefix from sysctl call for kern.osrelease")
	}

	buildPrefix, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("converting build prefix from string to int: %w", err)
	}

	return buildPrefix, nil
}
