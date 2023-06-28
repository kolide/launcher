package checkups

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/kolide/launcher/pkg/autoupdate/tuf"
)

type Osquery struct {
	UpdateChannel string
	TufServerURL  string
	OsquerydPath  string
}

func (c *Osquery) Name() string {
	return "Osquery"
}

func (c *Osquery) Run(short io.Writer) (string, error) {
	return checkupOsquery(short, c.UpdateChannel, c.TufServerURL, c.OsquerydPath)
}

// checkupOsquery tests for the presence of files important to osquery
func checkupOsquery(short io.Writer, updateChannel, tufServerURL, osquerydPath string) (string, error) {
	if osquerydPath == "" {
		return "", fmt.Errorf("osqueryd path unknown")
	}

	_, err := os.Stat(osquerydPath)
	if err != nil {
		return "", fmt.Errorf("osqueryd does not exist")
	}

	osqueryArgs := []string{"--version"}
	cmd := exec.CommandContext(context.TODO(), osquerydPath, osqueryArgs...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error occurred while querying osquery version output %s: err: %w", out, err)
	}

	currentVersion := strings.TrimLeft(string(out), fmt.Sprintf("%s version ", windowsAddExe("osqueryd")))
	currentVersion = strings.TrimRight(currentVersion, "\n")

	info(short, fmt.Sprintf("Current version:\t%s", currentVersion))

	// Query the TUF repo for what the target version of osquery is
	targetVersion, err := tuf.GetChannelVersionFromTufServer("osqueryd", updateChannel, tufServerURL)
	if err != nil {
		return "", fmt.Errorf("failed to query TUF server: %w", err)
	}

	info(short, fmt.Sprintf("Target version:\t%s", targetVersion))
	return "Osquery version checks complete", nil
}

// windowsAddExe appends ".exe" to the input string when running on Windows
func windowsAddExe(in string) string {
	if runtime.GOOS == "windows" {
		return in + ".exe"
	}

	return in
}
