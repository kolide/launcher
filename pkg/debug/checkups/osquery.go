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
)

type osqueryCheckup struct {
	k              types.Knapsack
	status         Status
	executionTimes map[string]string // maps command to how long it took to run, in ms
	summary        string
}

func (o *osqueryCheckup) Name() string {
	return "Osquery"
}

func (o *osqueryCheckup) Run(ctx context.Context, extraWriter io.Writer) error {
	// Determine passing status by running osqueryd --version
	o.executionTimes = make(map[string]string)
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

	return nil
}

func (o *osqueryCheckup) version(ctx context.Context) (string, error) {
	osquerydPath := o.k.LatestOsquerydPath(ctx)

	cmdCtx, cmdCancel := context.WithTimeout(ctx, 10*time.Second)
	defer cmdCancel()

	cmd := exec.CommandContext(cmdCtx, osquerydPath, "--version")
	hideWindow(cmd)
	startTime := time.Now().UnixMilli()
	out, err := cmd.CombinedOutput()
	o.executionTimes[cmd.String()] = fmt.Sprintf("%d ms", time.Now().UnixMilli()-startTime)
	if err != nil {
		return "", fmt.Errorf("running %s version: err %w, output %s", osquerydPath, err, string(out))
	}

	return strings.TrimSpace(string(out)), nil
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

	cmd := exec.CommandContext(cmdCtx, launcherPath, "interactive")
	hideWindow(cmd)
	cmd.Stdin = strings.NewReader(`select * from osquery_info;`)

	startTime := time.Now().UnixMilli()
	out, err := cmd.CombinedOutput()
	o.executionTimes[cmd.String()] = fmt.Sprintf("%d ms", time.Now().UnixMilli()-startTime)
	if err != nil {
		return fmt.Errorf("running %s interactive: err %w, output %s", launcherPath, err, string(out))
	}

	return nil
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
	return o.executionTimes
}
