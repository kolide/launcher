package checkups

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kolide/launcher/pkg/allowedpaths"
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
	if runtime.GOOS != "darwin" {
		return ""
	}

	return "Launchd"
}

func (c *launchdCheckup) Run(ctx context.Context, extraWriter io.Writer) error {
	// Check that the plist exists (this uses open not stat, because we also want to copy it)
	launchdPlist, err := os.Open(launchdPlistPath)
	if os.IsNotExist(err) {
		c.status = Failing
		c.summary = "plist does not exist"
		return nil
	} else if err != nil {
		c.status = Failing
		c.summary = fmt.Sprintf("error reading %s: %s", launchdPlistPath, err)
		return nil
	}
	defer launchdPlist.Close()

	extraZip := zip.NewWriter(extraWriter)
	defer extraZip.Close()

	zippedPlist, err := extraZip.Create(filepath.Base(launchdPlistPath))
	if err != nil {
		c.status = Erroring
		c.summary = fmt.Sprintf("unable to create extra information: %s", err)
		return nil
	}

	if _, err := io.Copy(zippedPlist, launchdPlist); err != nil {
		c.status = Erroring
		c.summary = fmt.Sprintf("unable to write extra information: %s", err)
		return nil
	}

	// run launchctl to check status
	var printOut bytes.Buffer

	cmd, err := allowedpaths.Launchctl(ctx, "print", launchdServiceName)
	if err != nil {
		c.status = Erroring
		c.summary = fmt.Sprintf("unable to create launchctl command: %s", err)
		return nil
	}

	cmd.Stdout = &printOut
	cmd.Stderr = &printOut
	if err := cmd.Run(); err != nil {
		c.status = Failing
		c.summary = fmt.Sprintf("error running launchctl print: %s", err)
		return nil
	}

	zippedOut, err := extraZip.Create("launchctl-print.txt")
	if err != nil {
		c.status = Erroring
		c.summary = fmt.Sprintf("unable to create launchctl-print.txt: %s", err)
		return nil
	}
	if _, err := zippedOut.Write(printOut.Bytes()); err != nil {
		c.status = Erroring
		c.summary = fmt.Sprintf("unable to write launchctl-print.txt: %s", err)
		return nil

	}

	if !strings.Contains(printOut.String(), "state = running") {
		c.status = Failing
		c.summary = fmt.Sprintf("state not active")
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
