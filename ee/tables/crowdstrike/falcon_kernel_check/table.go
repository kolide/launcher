//go:build linux
// +build linux

package falcon_kernel_check

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

type Table struct {
	slogger *slog.Logger
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("kernel"),
		table.IntegerColumn("supported"),
		table.IntegerColumn("sensor_version"),
	}

	tableName := "kolide_falcon_kernel_check"

	t := &Table{
		slogger: slogger.With("table", tableName),
	}

	return tablewrapper.New(flags, slogger, tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", "kolide_falcon_kernel_check")
	defer span.End()

	output, err := tablehelpers.RunSimple(ctx, t.slogger, 5, allowedcmd.FalconKernelCheck, []string{})
	if err != nil {
		t.slogger.Log(ctx, slog.LevelInfo,
			"exec failed",
			"err", err,
		)
		return nil, err
	}

	status, err := parseStatus(string(output))
	if err != nil {
		t.slogger.Log(ctx, slog.LevelInfo,
			"error parsing exec status",
			"err", err,
		)
		return nil, err
	}

	results := []map[string]string{status}

	return results, nil
}

// Example falcon-kernel-check output:

// $ sudo /opt/CrowdStrike/falcon-kernel-check
// Host OS 5.13.0-51-generic #58~20.04.1-Ubuntu SMP Tue Jun 14 11:29:12 UTC 2022 is supported by Sensor version 14006.

// # Upgrade happens
// $ sudo /opt/CrowdStrike/falcon-kernel-check
// Host OS Linux 5.15.0-46-generic #49~20.04.1-Ubuntu SMP Thu Aug 4 19:15:44 UTC 2022 is not supported by Sensor version 14006.
//
// This regexp gets matches for the kernel string, supported status, and sensor version number
var kernelCheckRegexp = regexp.MustCompile(`^((?:Host OS (.*) (is supported|is not supported)))(?: by Sensor version (\d*))`)

func parseStatus(status string) (map[string]string, error) {
	matches := kernelCheckRegexp.FindAllStringSubmatch(status, -1)
	if len(matches) != 1 {
		return nil, fmt.Errorf("Failed to match output: %s", status)
	}
	if len(matches[0]) != 5 {
		return nil, fmt.Errorf("Got %d matches. Expected 5. Failed to match output: %s", len(matches[0]), status)
	}

	// matches[0][2] = kernel version string
	// matches[0][3] = (is supported|is not supported)
	// matches[0][4] = sensor version number
	supported := "0"
	if matches[0][3] == "is supported" {
		supported = "1"
	}

	data := make(map[string]string, 3)
	data["kernel"] = matches[0][2]
	data["supported"] = supported
	data["sensor_version"] = matches[0][4]

	return data, nil
}
