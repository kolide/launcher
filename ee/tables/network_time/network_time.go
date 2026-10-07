//go:build darwin

package network_time

import (
	"context"
	"errors"
	"log/slog"

	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/kolide/launcher/v2/ee/allowedcmd"
	"github.com/kolide/launcher/v2/ee/observability"
	"github.com/kolide/launcher/v2/ee/tables/tablehelpers"
	"github.com/kolide/launcher/v2/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

const (
	tableName = "kolide_network_time"
)

type setting struct {
	columnName   string
	arg          string
	outputPrefix string
	isState      bool
}

var settings = []setting{
	{
		columnName:   "using_network_time",
		arg:          "-getusingnetworktime",
		outputPrefix: "Network Time:",
		isState:      true,
	},
	{
		columnName:   "network_time_server",
		arg:          "-getnetworktimeserver",
		outputPrefix: "Network Time Server:",
	},
}

type networkTimeExecer func(ctx context.Context, slogger *slog.Logger) ([]byte, error)

type NetworkTime struct {
	slogger      *slog.Logger
	execFunction networkTimeExecer
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.IntegerColumn("using_network_time"),
		table.TextColumn("network_time_server"),
	}

	networkTimeTable := &NetworkTime{
		slogger:      slogger.With("table", tableName),
		execFunction: networkTimeExec,
	}

	return tablewrapper.New(flags, slogger, tableName, columns, networkTimeTable.generateNetworkTime,
		tablewrapper.WithDescription("macOS network time settings from `systemsetup`, including whether the system clock is synchronized against a network time server, and which server it uses."),
	)
}

func networkTimeExec(ctx context.Context, slogger *slog.Logger) ([]byte, error) {
	args := make([]string, 0, len(settings))
	for _, setting := range settings {
		args = append(args, setting.arg)
	}

	return tablehelpers.RunSimple(ctx, slogger, 10, allowedcmd.Systemsetup, args)
}

func (t *NetworkTime) generateNetworkTime(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", tableName)
	defer span.End()

	return generateNetworkTimeData(ctx, t.execFunction, t.slogger)
}

func generateNetworkTimeData(ctx context.Context, execFunction networkTimeExecer, slogger *slog.Logger) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	results := make([]map[string]string, 0)

	output, err := execFunction(ctx, slogger)
	if err != nil {
		// log that the binary doesn't exist, but don't return an error
		if errors.Is(err, allowedcmd.ErrCommandNotFound) {
			slogger.Log(ctx, slog.LevelWarn,
				"systemsetup binary not found",
				"err", err,
			)
			return nil, nil
		}

		slogger.Log(ctx, slog.LevelError,
			"systemsetup failed",
			"err", err,
		)
		return results, nil
	}

	parsed := parseSystemsetupOutput(output)

	if len(parsed) == 0 {
		slogger.Log(ctx, slog.LevelWarn,
			"no settings in systemsetup output",
			"output", string(output),
		)
		return results, nil
	}

	results = append(results, parsed)

	return results, nil
}
