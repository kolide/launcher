package checkups

import (
	"context"
	"errors"
	"io"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/osquery/runtime/history"
)

type (
	osqRestartCheckup struct {
		k       types.Knapsack
		status  Status
		summary string
		data    map[string]any
	}
)

func (orc *osqRestartCheckup) Data() any             { return orc.data }
func (orc *osqRestartCheckup) ExtraFileName() string { return "" }
func (orc *osqRestartCheckup) Name() string          { return "Osquery Restarts" }
func (orc *osqRestartCheckup) Status() Status        { return orc.status }
func (orc *osqRestartCheckup) Summary() string       { return orc.summary }

func (orc *osqRestartCheckup) Run(ctx context.Context, extraFH io.Writer) error {
	orc.data = make(map[string]any)

	restartHistory, err := history.GetHistory()
	if err != nil && errors.Is(err, history.NoInstancesError{}) {
		orc.status = Informational
		orc.summary = "No osquery restart history instances available"
		return nil
	}

	if err != nil {
		orc.status = Erroring
		orc.summary = "Unable to collect osquery restart history"
		orc.data["error"] = err.Error()
		return nil
	}

	results := make([]map[string]string, len(restartHistory))

	for idx, instance := range restartHistory {
		results[idx] = map[string]string{
			"registration_id": instance.RegistrationId,
			"instance_run_id": instance.RunId,
			"start_time":      instance.StartTime,
			"connect_time":    instance.ConnectTime,
			"exit_time":       instance.ExitTime,
			"instance_id":     instance.InstanceId,
			"version":         instance.Version,
			"errors":          instance.Error,
		}
	}

	orc.status = Passing
	orc.data["history"] = results
	orc.summary = "Successfully collected osquery restart history"

	return nil
}
