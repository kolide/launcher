package checkups

import (
	"archive/zip"
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
		o.summary = err.Error()
		return fmt.Errorf("running osqueryd version: %w", err)
	} else {
		o.status = Passing
		o.summary = osqueryVersion
	}

	// Run launcher interactive to capture timing details
	if err := o.interactive(ctx); err != nil {
		return fmt.Errorf("running launcher interactive: %w", err)
	}

	// If we're running doctor and not flare, that's all we need
	if extraWriter == io.Discard {
		return nil
	}

	zipWriter := zip.NewWriter(extraWriter)
	defer zipWriter.Close()

	if err := o.foreground(ctx, zipWriter); err != nil {
		return fmt.Errorf("running osqueryd in foreground: %w", err)
	}

	return nil
}

func (o *osqueryCheckup) version(ctx context.Context) (string, error) {
	osquery := osquerydPath()

	cmdCtx, cmdCancel := context.WithTimeout(ctx, 10*time.Second)
	defer cmdCancel()

	cmd := exec.CommandContext(cmdCtx, osquery, "--version")
	startTime := time.Now().UnixMilli()
	out, err := cmd.CombinedOutput()
	o.executionTimes[strings.Join(cmd.Args, " ")] = fmt.Sprintf("%d ms", time.Now().UnixMilli()-startTime)
	if err != nil {
		return "", fmt.Errorf("running %s version: err %w, output %s", osquery, err, string(out))
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
	o.executionTimes[strings.Join(cmd.Args, " ")] = fmt.Sprintf("%d ms", time.Now().UnixMilli()-startTime)
	if err != nil {
		return fmt.Errorf("running %s interactive: err %w, output %s", launcherPath, err, string(out))
	}

	return nil
}

func (o *osqueryCheckup) foreground(ctx context.Context, zipWriter *zip.Writer) error {
	osquery := osquerydPath()

	runInForegroundDuration := 10 * time.Second

	cmdCtx, cmdCancel := context.WithTimeout(ctx, runInForegroundDuration)
	defer cmdCancel()

	out, err := zipWriter.Create("osqueryd-foreground.log")
	if err != nil {
		return fmt.Errorf("creating zip file for stderr: %w", err)
	}
	cmd := exec.CommandContext(cmdCtx, osquery, "--ephemeral", "--disable_database", "--disable_logging", "--verbose")
	cmd.Stderr = out

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting osqueryd in foreground: err %w", err)
	}

	time.Sleep(runInForegroundDuration)

	return nil
}

func osquerydPath() string {
	switch runtime.GOOS {
	case "linux", "darwin":
		return "/usr/local/kolide-k2/bin/osqueryd"
	case "windows":
		return `C:\Program Files\Kolide\Launcher-kolide-k2\bin\osqueryd.exe`
	}

	return ""
}

func (o *osqueryCheckup) ExtraFileName() string {
	return "osqueryd-output.zip"
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
