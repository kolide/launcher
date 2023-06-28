package checkups

import (
	"fmt"
	"io"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/autoupdate/tuf"
)

type Version struct {
	UpdateChannel string
	TufServerURL  string
	Version       version.Info
}

func (c *Version) Name() string {
	return "Check launcher version"
}

func (c *Version) Run(short io.Writer) (string, error) {
	return checkupVersion(short, c.UpdateChannel, c.TufServerURL, c.Version)
}

// checkupVersion tests to see if the current launcher version is up to date
func checkupVersion(short io.Writer, updateChannel, tufServerURL string, v version.Info) (string, error) {
	info(short, fmt.Sprintf("Update Channel:\t%s", updateChannel))
	info(short, fmt.Sprintf("TUF Server:\t%s", tufServerURL))
	info(short, fmt.Sprintf("Current version:\t%s", v.Version))

	// Query the TUF repo for what the target version of launcher is
	targetVersion, err := tuf.GetChannelVersionFromTufServer("launcher", updateChannel, tufServerURL)
	if err != nil {
		return "", fmt.Errorf("Failed to query TUF server: %w", err)
	}

	info(short, fmt.Sprintf("Target version:\t%s", targetVersion))
	return "Launcher version checks complete", nil
}
