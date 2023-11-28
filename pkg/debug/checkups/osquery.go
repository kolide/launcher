package checkups

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/osquery/runtime/history"
)

type osqueryCheckup struct {
	k       types.Knapsack
	status  Status
	data    map[string]any
	summary string
}

func (o *osqueryCheckup) Name() string {
	return "Osquery"
}

func (o *osqueryCheckup) Run(ctx context.Context, extraWriter io.Writer) error {
	o.data = make(map[string]any)

	// Determine passing status by running osqueryd --version
	if osqueryVersion, err := o.version(ctx); err != nil {
		o.status = Failing
		return fmt.Errorf("running osqueryd version: %w", err)
	} else {
		o.status = Passing
		o.summary = osqueryVersion
	}

	// Run launcher interactive to capture timing details
	if err := o.interactive(ctx); err != nil {
		return fmt.Errorf("running launcher interactive: %w", err)
	}

	// Retrieve osquery instance history to see if we have an abnormal number of restarts
	o.instanceHistory()

	// Check to see if current extension is healthy
	o.extensionHealth()

	return nil
}

func (o *osqueryCheckup) version(ctx context.Context) (string, error) {
	osquerydPath := o.k.LatestOsquerydPath(ctx)
	o.data["osqueryd_path"] = osquerydPath

	cmdCtx, cmdCancel := context.WithTimeout(ctx, 10*time.Second)
	defer cmdCancel()

	cmd := exec.CommandContext(cmdCtx, osquerydPath, "--version") //nolint:forbidigo // We trust the autoupdate library to find the correct path
	hideWindow(cmd)
	startTime := time.Now().UnixMilli()
	out, err := cmd.CombinedOutput()
	o.data["execution_time_osq_version"] = fmt.Sprintf("%d ms", time.Now().UnixMilli()-startTime)
	if err != nil {
		return "", fmt.Errorf("running %s version: err %w, output %s", osquerydPath, err, string(out))
	}

	osqVersion := strings.TrimSpace(string(out))
	o.data["osqueryd_version"] = osqVersion

	return osqVersion, nil
}

func (o *osqueryCheckup) interactive(ctx context.Context) error {
	var launcherPath string
	switch runtime.GOOS {
	case "linux", "darwin":
		launcherPath = "/usr/local/kolide-k2/bin/launcher"
	case "windows":
		launcherPath = `C:\Program Files\Kolide\Launcher-kolide-k2\bin\launcher.exe`
	}

	cmdCtx, cmdCancel := context.WithTimeout(ctx, 20*time.Second)
	defer cmdCancel()

	// We trust the autoupdate library to find the correct path
	cmd := exec.CommandContext(cmdCtx, launcherPath, "interactive") //nolint:forbidigo // We trust the autoupdate library to find the correct path
	hideWindow(cmd)
	cmd.Stdin = strings.NewReader(`select * from osquery_info;`)

	startTime := time.Now().UnixMilli()
	out, err := cmd.CombinedOutput()
	o.data["execution_time_launcher_interactive"] = fmt.Sprintf("%d ms", time.Now().UnixMilli()-startTime)
	if err != nil {
		return fmt.Errorf("running %s interactive: err %w, output %s", launcherPath, err, string(out))
	}

	return nil
}

func (o *osqueryCheckup) instanceHistory() {
	mostRecentInstances, err := history.GetHistory()
	if err != nil {
		o.data["osquery_instance_history"] = fmt.Errorf("could not get instance history: %+v", err)
		return
	}

	mostRecentInstancesFormatted := make([]map[string]string, len(mostRecentInstances))
	for i, instance := range mostRecentInstances {
		mostRecentInstancesFormatted[i] = map[string]string{
			"start_time":   instance.StartTime,
			"connect_time": instance.ConnectTime,
			"exit_time":    instance.ExitTime,
			"hostname":     instance.Hostname,
			"instance_id":  instance.InstanceId,
			"version":      instance.Version,
			"error":        instance.Error,
		}

	}

	o.data["osquery_instance_history"] = mostRecentInstancesFormatted
}

func (o *osqueryCheckup) extensionHealth() {
	err := o.k.QuerierHealthy()
	if err != nil {
		o.data["osquery_instance_healthcheck_err"] = err.Error()
	} else {
		o.data["osquery_instance_healthcheck_err"] = nil
	}
}

func (o *osqueryCheckup) ExtraFileName() string {
	return ""
}

func (o *osqueryCheckup) Status() Status {
	return o.status
}

func (o *osqueryCheckup) Summary() string {
	return o.summary
}

func (o *osqueryCheckup) Data() any {
	return o.data
}
