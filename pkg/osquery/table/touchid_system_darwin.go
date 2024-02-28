package table

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/osquery/osquery-go/plugin/table"
)

func TouchIDSystemConfig(slogger *slog.Logger) *table.Plugin {
	t := &touchIDSystemConfigTable{
		slogger: slogger.With("table", "kolide_touchid_system_config"),
	}
	columns := []table.ColumnDefinition{
		table.IntegerColumn("touchid_compatible"),
		table.TextColumn("secure_enclave_cpu"),
		table.IntegerColumn("touchid_enabled"),
		table.IntegerColumn("touchid_unlock"),
	}

	return table.NewPlugin("kolide_touchid_system_config", columns, t.generate)
}

type touchIDSystemConfigTable struct {
	slogger *slog.Logger
}

// TouchIDSystemConfigGenerate will be called whenever the table is queried.
func (t *touchIDSystemConfigTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var results []map[string]string
	var touchIDCompatible, secureEnclaveCPU, touchIDEnabled, touchIDUnlock string

	// Read the security chip from system_profiler
	var stdout bytes.Buffer
	cmd, err := allowedcmd.SystemProfiler(ctx, "SPiBridgeDataType")
	if err != nil {
		t.slogger.Log(ctx, slog.LevelDebug,
			"could not create system_profiler command",
			"err", err,
		)
		return results, nil
	}
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("calling system_profiler: %w", err)
	}

	r := regexp.MustCompile(` (?P<chip>T\d) `) // Matching on: Apple T[1|2] Security Chip
	match := r.FindStringSubmatch(stdout.String())
	if len(match) == 0 {
		secureEnclaveCPU = ""
	} else {
		secureEnclaveCPU = match[1]
	}

	// Read the system's bioutil configuration
	stdout.Reset()
	cmd, err = allowedcmd.Bioutil(ctx, "-r", "-s")
	if err != nil {
		t.slogger.Log(ctx, slog.LevelDebug,
			"could not create bioutil command",
			"err", err,
		)
		return results, nil
	}
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("calling bioutil for system configuration: %w", err)
	}
	configOutStr := stdout.String()
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
