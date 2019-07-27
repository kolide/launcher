package table

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"

	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

func TouchIDSystemConfig(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	t := &touchIDSystemConfigTable{
		client: client,
		logger: logger,
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
	client *osquery.ExtensionManagerClient
	logger log.Logger
	config *touchIDSystemConfig
}

type touchIDSystemConfig struct {
	touchIDCompatible int
	secureEnclaveCPU  string
	touchIDEnabled    int
	touchIDUnlock     int
}

// TouchIDSystemConfigGenerate will be called whenever the table is queried.
func (t *touchIDSystemConfigTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string
	var touchIDCompatible, secureEnclaveCPU, touchIDEnabled, touchIDUnlock string

	// Read the security chip from system_profiler
	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, "/usr/sbin/system_profiler", "SPiBridgeDataType")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, errors.Wrap(err, "calling system_profiler")
	}

	r := regexp.MustCompile(` (?P<chip>T\d) `) // Matching on: Apple T[1|2] Security Chip
	match := r.FindStringSubmatch(string(stdout.Bytes()))
	if len(match) == 0 {
		touchIDCompatible = "0"
		secureEnclaveCPU = ""
	} else {
		touchIDCompatible = "1"
		secureEnclaveCPU = match[1]
	}

	// Read the system's bioutil configuration
	stdout.Reset()
	cmd = exec.CommandContext(ctx, "/usr/bin/bioutil", "-r", "-s")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, errors.Wrap(err, "calling bioutil for system configuration")
	}
	configOutStr := string(stdout.Bytes())
	configSplit := strings.Split(configOutStr, ":")
	touchIDEnabled = configSplit[2][1:2]
	touchIDUnlock = configSplit[3][1:2]

	result := map[string]string{
		"touchid_compatible": touchIDCompatible,
		"secure_enclave_cpu": secureEnclaveCPU,
		"touchid_enabled":    touchIDEnabled,
		"touchid_unlock":     touchIDUnlock,
	}
	results = append(results, result)
	return results, nil
}
