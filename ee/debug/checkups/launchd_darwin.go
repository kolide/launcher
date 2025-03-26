//go:build darwin
// +build darwin

package checkups

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/kolide/launcher/ee/allowedcmd"
)

const (
	launchdPlistPath   = "/Library/LaunchDaemons/com.kolide-k2.launcher.plist"
	launchdServiceName = "system/com.kolide-k2.launcher"
)

type launchdCheckup struct {
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
