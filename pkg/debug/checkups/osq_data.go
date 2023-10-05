package checkups

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/kolide/launcher/pkg/agent/types"
)

const osSqlQuery = `
SELECT
	os_version.build as os_build,
	os_version.name as os_name,
	os_version.version as os_version,
	system_info.hardware_model,
	system_info.hardware_serial,
	system_info.hardware_vendor,
	system_info.uuid as hardware_uuid
FROM
	os_version, system_info;
`

type (
	osqResp         []map[string]string
	osqDataCollector struct {
		k       types.Knapsack
		status  Status
		summary string
		data    map[string]any
	}
)

func (odc *osqDataCollector) Data() map[string]any  { return odc.data }
func (odc *osqDataCollector) ExtraFileName() string { return "" }
func (odc *osqDataCollector) Name() string          { return "Host Info" }
func (odc *osqDataCollector) Status() Status        { return odc.status }
func (odc *osqDataCollector) Summary() string       { return odc.summary }

func (odc *osqDataCollector) Run(ctx context.Context, extraFH io.Writer) error {
	if result, err := odc.osqueryInteractive(ctx, osSqlQuery); err != nil {
		odc.data["osq_data"] = err.Error()
	} else {
		odc.data["osq_data"] = result
		if uuid, ok := result["hardware_uuid"]; ok { // append the uuid to summary line if we have it
			odc.summary = fmt.Sprintf("%s, uuid: %s", odc.summary, uuid)
		}
	}

	return nil
}

// osqueryInteractive execs osquery and parses the output to gather some basic host info.
// it was done this way to avoid bringing Querier into knapsack for a task that will only be run
// during flare or doctor
func (odc *osqDataCollector) osqueryInteractive(ctx context.Context, query string) (map[string]string, error) {
	cmdCtx, cmdCancel := context.WithTimeout(ctx, 20*time.Second)
	defer cmdCancel()

	osqPath := odc.k.LatestOsquerydPath(ctx)
	cmd := exec.CommandContext(cmdCtx, osqPath, "-S", "--json")
	hideWindow(cmd)
	cmd.Stdin = strings.NewReader(query)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running %s interactive: err %w, output %s", osqPath, err, string(out))
	}

	results := make(osqResp, 0)
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("parsing %s interactive: err %w, output %s", osqPath, err, string(out))
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("got empty response from %s interactive: output %s", osqPath, string(out))
	}

	return results[0], nil
}
