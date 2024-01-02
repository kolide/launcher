package checkups

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/osquery/runsimple"
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
	osqResp          []map[string]string
	osqDataCollector struct {
		k       types.Knapsack
		status  Status
		summary string
		data    map[string]any
	}
)

func (odc *osqDataCollector) Data() any             { return odc.data }
func (odc *osqDataCollector) ExtraFileName() string { return "" }
func (odc *osqDataCollector) Name() string          { return "Osquery Data" }
func (odc *osqDataCollector) Status() Status        { return odc.status }
func (odc *osqDataCollector) Summary() string       { return odc.summary }

func (odc *osqDataCollector) Run(ctx context.Context, extraFH io.Writer) error {
	odc.data = make(map[string]any)

	result, err := odc.queryData(ctx, osSqlQuery)
	if err != nil {
		odc.status = Erroring
		odc.data["error"] = err.Error()
		odc.summary = fmt.Sprintf("ERROR using osq interactive: %s", err.Error())
		return nil
	}

	data := make([]string, 0)
	for k, v := range result {
		if k != "" {
			data = append(data, fmt.Sprintf("%s: %s", k, v))
			odc.data[k] = v
		}
	}

	odc.status = Passing
	odc.summary = strings.Join(data, ", ")

	return nil
}

// queryData sets up a runsimple osq process and parses the output to gather some basic host info.
// it was done this way to avoid bringing Querier into knapsack for a task that will only be run
// during flare or doctor
func (odc *osqDataCollector) queryData(ctx context.Context, query string) (map[string]string, error) {
	osqPath := odc.k.LatestOsquerydPath(ctx)
	var resultBuffer bytes.Buffer
	osqCtx, cmdCancel := context.WithTimeout(ctx, 5*time.Second)
	defer cmdCancel()

	osq, err := runsimple.NewOsqueryProcess(osqPath, runsimple.WithStdout(&resultBuffer))
	if err != nil {
		return nil, fmt.Errorf("unable to create osq process %w", err)
	}

	if sqlErr := osq.RunSql(osqCtx, []byte(query)); osqCtx.Err() != nil {
		return nil, fmt.Errorf("osq data query invocation context error: %w", osqCtx.Err())
	} else if sqlErr != nil {
		return nil, fmt.Errorf("osq data SQL error: %w", sqlErr)
	}

	queryResponse := resultBuffer.Bytes()

	results := make(osqResp, 0)
	if err := json.Unmarshal(queryResponse, &results); err != nil {
		return nil, fmt.Errorf("unable to parse osq data query results from output %s. error: %w", string(queryResponse), err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("got empty result set from osq_data query: output %s", string(queryResponse))
	}

	return results[0], nil
}
