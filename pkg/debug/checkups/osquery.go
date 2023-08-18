package checkups

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type osqueryCheckup struct {
	status         Status
	executionTimes map[string]int64
	summary        string
}

func (o *osqueryCheckup) Name() string {
	return "Osquery"
}

func (o *osqueryCheckup) Run(ctx context.Context, extraWriter io.Writer) error {
	o.executionTimes = make(map[string]int64)
	if osqueryVersion, err := o.version(ctx); err != nil {
		o.status = Failing
		o.summary = err.Error()
		return err
	} else {
		o.status = Passing
		o.summary = osqueryVersion
	}

	if err := o.interactive(ctx); err != nil {
		return err
	}

	return nil
}

func (o *osqueryCheckup) version(ctx context.Context) (string, error) {
	var osqueryPath string
	switch runtime.GOOS {
	case "linux", "darwin":
		osqueryPath = "/usr/local/kolide-k2/bin/osqueryd"
	case "windows":
		osqueryPath = `C:\Program Files\Kolide\Launcher-kolide-k2\bin\osqueryd.exe`
	}

	cmdCtx, cmdCancel := context.WithTimeout(ctx, 10*time.Second)
	defer cmdCancel()

	cmd := exec.CommandContext(cmdCtx, osqueryPath, "--version")
	startTime := time.Now().UnixMilli()
	out, err := cmd.CombinedOutput()
	o.executionTimes[strings.Join(cmd.Args, " ")] = time.Now().UnixMilli() - startTime
	if err != nil {
		return "", fmt.Errorf("running %s version: err %w, output %s", osqueryPath, err, string(out))
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

	cmdCtx, cmdCancel := context.WithTimeout(ctx, 10*time.Second)
	defer cmdCancel()

	cmd := exec.CommandContext(cmdCtx, launcherPath, "interactive")
	cmd.Stdin = strings.NewReader("select * from osquery_info;")

	startTime := time.Now().UnixMilli()
	out, err := cmd.CombinedOutput()
	o.executionTimes[strings.Join(cmd.Args, " ")] = time.Now().UnixMilli() - startTime
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
