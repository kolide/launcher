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

type InitLogs struct {
	status  Status
	summary string
}

func (c *InitLogs) Name() string {
	return "InitLogs"
}

func (c *InitLogs) Run(ctx context.Context, fullFH io.Writer) error {
	c.status = Passing

	// if were discarding, just return
	if fullFH == io.Discard {
		return nil
	}

	logZip := zip.NewWriter(fullFH)
	defer logZip.Close()

	switch runtime.GOOS {
	case "darwin":
		writeDarwinInitLogs(logZip)
	case "windows":
		writeWindowsInitLogs(ctx, logZip)
	}

	return nil
}

func (c *InitLogs) Status() Status {
	return c.status
}

func (c *InitLogs) Summary() string {
	return c.summary
}

func (c *InitLogs) ExtraFileName() string {
	return "init_logs.zip"
}

func (c *InitLogs) Data() any {
	return nil
}

func writeDarwinInitLogs(logZip *zip.Writer) {
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

func writeWindowsInitLogs(ctx context.Context, logZip *zip.Writer) {
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
