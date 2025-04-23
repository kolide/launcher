package table

import (
	"context"
	"errors"
	"log/slog"
	"os/user"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

func TouchIDUserConfig(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	t := &touchIDUserConfigTable{
		slogger: slogger.With("table", "kolide_touchid_user_config"),
	}
	columns := []table.ColumnDefinition{
		table.IntegerColumn("uid"),
		table.IntegerColumn("fingerprints_registered"),
		table.IntegerColumn("touchid_unlock"),
		table.IntegerColumn("touchid_applepay"),
		table.IntegerColumn("effective_unlock"),
		table.IntegerColumn("effective_applepay"),
	}

	return tablewrapper.New(flags, slogger, "kolide_touchid_user_config", columns, t.generate)
}

type touchIDUserConfigTable struct {
	slogger *slog.Logger
}

func (t *touchIDUserConfigTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", "kolide_touchid_user_config")
	defer span.End()

	q := queryContext.Constraints["uid"]
	if len(q.Constraints) == 0 {
		t.slogger.Log(ctx, slog.LevelDebug,
			"table requires a uid constraint, but none provided",
		)
		return nil, errors.New("The touchid_user_config table requires that you specify a constraint WHERE uid =")
	}

	var results []map[string]string
	for _, constraint := range q.Constraints {
		var touchIDUnlock, touchIDApplePay, effectiveUnlock, effectiveApplePay string

		// Verify the user exists on the system before proceeding
		u, err := user.LookupId(constraint.Expression)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelDebug,
				"nonexistent user",
				"uid", constraint.Expression,
				"err", err,
			)
			continue
		}

		// Get the user's TouchID config
		configOutput, err := tablehelpers.RunSimple(ctx, t.slogger, 10, allowedcmd.Bioutil, []string{"-r"}, tablehelpers.WithUid(u.Uid))
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"could not run bioutil -r",
				"uid", u.Uid,
				"err", err,
			)
			continue
		}
		configSplit := strings.Split(string(configOutput), ":")

		// If the length of the split is 2, TouchID is not configured for this user
		// Otherwise, extract the values from the split.
		if len(configSplit) == 2 {
			touchIDUnlock, touchIDApplePay, effectiveUnlock, effectiveApplePay = "0", "0", "0", "0"
		} else if len(configSplit) == 6 {
			touchIDUnlock = configSplit[2][1:2]
			touchIDApplePay = configSplit[3][1:2]
			effectiveUnlock = configSplit[4][1:2]
			effectiveApplePay = configSplit[5][1:2]
		} else {
			t.slogger.Log(ctx, slog.LevelDebug,
				"bioutil -r returned unexpected output",
				"uid", u.Uid,
				"output", string(configOutput),
			)
			continue
		}

		// Grab the fingerprint count
		countOut, err := tablehelpers.RunSimple(ctx, t.slogger, 10, allowedcmd.Bioutil, []string{"-c"}, tablehelpers.WithUid(u.Uid))
		if err != nil {
			t.slogger.Log(ctx, slog.LevelDebug,
				"could not run bioutil -c",
				"uid", u.Uid,
				"err", err,
			)
			continue
		}
		countSplit := strings.Split(string(countOut), ":")
		fingerprintCount := strings.ReplaceAll(countSplit[1], "\t", "")[:1]

		// If the fingerprint count is 0, set effective values to 0
		// This is due to a bug in `bioutil -r` incorrectly always returning 1
		// See https://github.com/kolide/launcher/pull/502#pullrequestreview-284351577
		if fingerprintCount == "0" {
			effectiveApplePay, effectiveUnlock = "0", "0"
		}

		result := map[string]string{
			"uid":                     u.Uid,
			"fingerprints_registered": fingerprintCount,
			"touchid_unlock":          touchIDUnlock,
			"touchid_applepay":        touchIDApplePay,
			"effective_unlock":        effectiveUnlock,
			"effective_applepay":      effectiveApplePay,
		}
		results = append(results, result)
	}

	return results, nil
}
