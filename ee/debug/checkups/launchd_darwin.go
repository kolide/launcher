//go:build darwin
// +build darwin

package checkups

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
)

const (
	launchdPlistPath   = "/Library/LaunchDaemons/com.kolide-k2.launcher.plist"
	launchdServiceName = "system/com.kolide-k2.launcher"
)

type launchdCheckup struct {
	k       types.Knapsack
	status  Status
	summary string
}

func (c *launchdCheckup) Name() string {
	return "Launchd"
}

func (c *launchdCheckup) Run(ctx context.Context, extraWriter io.Writer) error {
	// Check that the plist exists
	if _, err := os.Stat(launchdPlistPath); os.IsNotExist(err) {
		c.status = Failing
		c.summary = "plist does not exist"
		return nil
	}

	extraZip := zip.NewWriter(extraWriter)
	defer extraZip.Close()

	// Add plist file using our utility
	if err := addFileToZip(extraZip, launchdPlistPath); err != nil {
		c.status = Erroring
		c.summary = fmt.Sprintf("unable to add plist file: %s", err)
		return nil
	}

	// Run launchctl to check status
	cmd, err := allowedcmd.Launchctl(ctx, "print", launchdServiceName)
	if err != nil {
		c.status = Erroring
		c.summary = fmt.Sprintf("unable to create launchctl command: %s", err)
		return nil
	}

	output, err := cmd.Output()
	if err != nil {
		c.status = Failing
		c.summary = fmt.Sprintf("error running launchctl print: %s", err)
		return nil
	}

	// Add command output using our streaming utility
	if err := addStreamToZip(extraZip, "launchctl-print.txt", time.Now(), bytes.NewReader(output)); err != nil {
		c.status = Erroring
		c.summary = fmt.Sprintf("unable to add launchctl-print.txt output: %s", err)
		return nil
	}

	if !strings.Contains(string(output), "state = running") {
		c.status = Failing
		c.summary = "state not active"
		return nil
	}

	c.status = Passing
	c.summary = "state is running"

	// all done unless we're running flare
	if extraWriter == io.Discard {
		return nil
	}

	launchdLogBytes, err := gatherLaunchdLogs()
	if err != nil {
		launchdLogBytes = []byte(err.Error()) // add error as output for review if needed
	}

	if len(launchdLogBytes) == 0 {
		return nil
	}

	if err := addStreamToZip(extraZip, "launchd-kolide-logs.txt", time.Now(), bytes.NewReader(launchdLogBytes)); err != nil {
		// log the error if slogger is available but don't change summary for this
		if c.k.Slogger() != nil {
			c.k.Slogger().Log(context.Background(), slog.LevelDebug,
				"adding launchd logs to zip",
				"err", err,
			)
		}
	}

	return nil
}

func (c *launchdCheckup) ExtraFileName() string {
	return "launchd.zip"
}

func (c *launchdCheckup) Status() Status {
	return c.status
}

func (c *launchdCheckup) Summary() string {
	return c.summary
}

func (c *launchdCheckup) Data() any {
	return nil
}

func gatherLaunchdLogs() ([]byte, error) {
	matches, err := filepath.Glob("/var/log/com.apple.xpc.launchd/launchd.log*")
	if err != nil {
		return nil, fmt.Errorf("globbing launchd logfiles: %w", err)
	}

	var logBuffer bytes.Buffer
	for _, filename := range matches {
		file, err := os.Open(filename)
		if err != nil {
			return nil, fmt.Errorf("opening file '%s': %w", filename, err)
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			// our logs should all contain something like 'system/com.kolide-k2.launcher'
			if strings.Contains(line, "kolide") {
				logBuffer.WriteString(line + "\n")
			}
		}

		if err := scanner.Err(); err != nil {
			file.Close()
			return nil, fmt.Errorf("scanning file '%s': %w", filename, err)
		}

		file.Close()
	}

	return logBuffer.Bytes(), nil
}
