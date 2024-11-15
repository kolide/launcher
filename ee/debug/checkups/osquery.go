package checkups

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
)

type osqueryCheckup struct {
	k              types.Knapsack
	status         Status
	executionTimes map[string]any // maps command to how long it took to run, in ms
	summary        string
}

func (o *osqueryCheckup) Name() string {
	return "Osquery"
}

func (o *osqueryCheckup) Run(ctx context.Context, extraWriter io.Writer) error {
	// Determine passing status by running osqueryd --version
	o.executionTimes = make(map[string]any)
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

	cmd := exec.CommandContext(cmdCtx, osquerydPath, "--version") //nolint:forbidigo // We trust the autoupdate library to find the correct path
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
	launcherPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting current running executable: %w", err)
	}

	cmdCtx, cmdCancel := context.WithTimeout(ctx, 20*time.Second)
	defer cmdCancel()

	cmd := exec.CommandContext(cmdCtx, launcherPath, "interactive", "--osqueryd_path", o.k.LatestOsquerydPath(ctx)) //nolint:forbidigo // It is safe to exec the current running executable
	hideWindow(cmd)
	cmd.Stdin = strings.NewReader(`select * from osquery_info;`)

	startTime := time.Now().UnixMilli()
	out, err := cmd.CombinedOutput()
	o.executionTimes[cmd.String()] = fmt.Sprintf("%d ms", time.Now().UnixMilli()-startTime)
	if err != nil {
		return fmt.Errorf("running %s interactive: err %w, output %s; ctx err: %+v", launcherPath, err, string(out), cmdCtx.Err())
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
