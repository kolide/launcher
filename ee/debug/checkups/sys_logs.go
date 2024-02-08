package checkups

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type SysLogs struct {
	status  Status
	summary string
}

func (c *SysLogs) Name() string {
	return "SysLogs"
}

func (c *SysLogs) Run(ctx context.Context, fullFH io.Writer) error {
	c.status = Passing

	// if were discarding, just return
	if fullFH == io.Discard {
		return nil
	}

	logZip := zip.NewWriter(fullFH)
	defer logZip.Close()

	switch runtime.GOOS {
	case "darwin":
		writeDarwinSysLogs(logZip)
	case "windows":
		writeWindowsSysLogs(ctx, logZip)
	}

	return nil
}

func (c *SysLogs) Status() Status {
	return c.status
}

func (c *SysLogs) Summary() string {
	return c.summary
}

func (c *SysLogs) ExtraFileName() string {
	return "sys_logs.zip"
}

func (c *SysLogs) Data() any {
	return nil
}

func writeDarwinSysLogs(logZip *zip.Writer) {
	stdMatches, _ := filepath.Glob("/var/log/kolide-k2/*")

	for _, f := range stdMatches {
		out, err := logZip.Create(filepath.Base(f))

		if err != nil {
			continue
		}

		in, err := os.Open(f)
		if err != nil {
			fmt.Fprintf(out, "error reading file:\n%s", err)
			continue
		}
		defer in.Close()

		io.Copy(out, in)
	}
}

func writeWindowsSysLogs(ctx context.Context, logZip *zip.Writer) {
	cmdStr := `Get-WinEvent -FilterHashtable @{LogName='Application'; ProviderName='launcher'} | ConvertTo-Json`
	cmd := exec.CommandContext(ctx, "powershell", "-Command", cmdStr) //nolint:forbidigo // This is just a debugging tool, not launcher proper

	outFile, err := logZip.Create("windows_launcher_events.json")
	if err != nil {
		return
	}

	cmd.Stderr = outFile
	cmd.Stdout = outFile

	cmd.Run()
}
