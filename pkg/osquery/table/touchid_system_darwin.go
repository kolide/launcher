package table

import (
	"context"
	"log/slog"
	"regexp"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

func TouchIDSystemConfig(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	t := &touchIDSystemConfigTable{
		slogger: slogger.With("table", "kolide_touchid_system_config"),
	}
	columns := []table.ColumnDefinition{
		table.IntegerColumn("touchid_compatible"),
		table.TextColumn("secure_enclave_cpu"),
		table.IntegerColumn("touchid_enabled"),
		table.IntegerColumn("touchid_unlock"),
	}

	return tablewrapper.New(flags, slogger, "kolide_touchid_system_config", columns, t.generate)
}

type touchIDSystemConfigTable struct {
	slogger *slog.Logger
}

// TouchIDSystemConfigGenerate will be called whenever the table is queried.
func (t *touchIDSystemConfigTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", "kolide_touchid_system_config")
	defer span.End()

	var results []map[string]string
	var touchIDCompatible, secureEnclaveCPU, touchIDEnabled, touchIDUnlock string

	stdout, err := tablehelpers.RunSimple(ctx, t.slogger, 10, allowedcmd.SystemProfiler, []string{"SPiBridgeDataType"})
	if err != nil {
		t.slogger.Log(ctx, slog.LevelDebug,
			"execing system_profiler SPiBridgeDataType",
			"err", err,
		)
		return results, nil
	}

	// Read the security chip from system_profiler
	r := regexp.MustCompile(` (?P<chip>T\d) `) // Matching on: Apple T[1|2] Security Chip
	match := r.FindStringSubmatch(string(stdout))
	if len(match) == 0 {
		secureEnclaveCPU = ""
	} else {
		secureEnclaveCPU = match[1]
	}

	// Read the system's bioutil configuration
	stdout, err = tablehelpers.RunSimple(ctx, t.slogger, 10, allowedcmd.Bioutil, []string{"-r", "-s"})
	if err != nil {
		t.slogger.Log(ctx, slog.LevelDebug,
			"execing bioutil",
			"err", err,
		)
		return results, nil
	}

	configOutStr := string(stdout)
	configSplit := strings.Split(configOutStr, ":")
	if len(configSplit) >= 3 {
		touchIDCompatible = "1"
		touchIDEnabled = configSplit[2][1:2]
		touchIDUnlock = configSplit[3][1:2]
	}

	result := map[string]string{
		"touchid_compatible": touchIDCompatible,
		"secure_enclave_cpu": secureEnclaveCPU,
		"touchid_enabled":    touchIDEnabled,
		"touchid_unlock":     touchIDUnlock,
	}
	results = append(results, result)
	return results, nil
}
