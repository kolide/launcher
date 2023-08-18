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
	status  Status
	summary string
}

func (o *osqueryCheckup) Name() string {
	return "Osquery"
}

func (o *osqueryCheckup) Run(ctx context.Context, extraWriter io.Writer) error {
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
	out, err := cmd.CombinedOutput()
	if err != nil {
		o.status = Failing
		o.summary = fmt.Sprintf("could not run %s --version: output %s, error %+v", osqueryPath, string(out), err)
		return fmt.Errorf("running %s version: %w", osqueryPath, err)
	}

	versionOutput := strings.TrimSpace(string(out))
	o.status = Passing
	o.summary = fmt.Sprintf("%s (%s)", versionOutput, osqueryPath)

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
	return nil
}
