package checkups

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/pkg/launcher"
)

type BinaryDirectory struct {
	k       types.Knapsack
	status  Status
	summary string
}

func (c *BinaryDirectory) Name() string {
	return "Binary directory contents"
}

func (c *BinaryDirectory) Run(_ context.Context, extraFH io.Writer) error {
	bindir := c.getBinDir()
	if bindir == "" {
		return errors.New("no default bin directory")
	}

	// Note that we're recursing `/usr/local/kolide-k2` and not .../bin. So the counts may not be what
	// you expect. (But the flare output is better)
	filecount, err := recursiveDirectoryContents(extraFH, bindir)

	switch {
	case errors.Is(err, os.ErrNotExist):
		c.status = Failing
		c.summary = fmt.Sprintf("binary directory (%s) not present", bindir)
	case err != nil:
		c.status = Erroring
		c.summary = fmt.Sprintf("listing files in binary directory (%s): %s", bindir, err)
	case filecount == 0:
		c.status = Warning
		c.summary = fmt.Sprintf("binary directory (%s) empty", bindir)
	default:
		c.status = Passing
		c.summary = fmt.Sprintf("binary directory (%s) contains %d files", bindir, filecount)
	}
	return nil
}

func (c *BinaryDirectory) ExtraFileName() string {
	return "file-list"
}

func (c *BinaryDirectory) Status() Status {
	return c.status
}

func (c *BinaryDirectory) Summary() string {
	return c.summary
}

func (c *BinaryDirectory) Data() any {
	return nil
}

// getBinDir returns the platform default binary directory. It should probably get folded into flags, but I'm not
// quite sure how yet.
func (c *BinaryDirectory) getBinDir() string {
	identifier := launcher.DefaultLauncherIdentifier
	if c.k.Identifier() != "" {
		identifier = c.k.Identifier()
	}

	switch runtime.GOOS {
	case "darwin":
		return fmt.Sprintf("/usr/local/%s", identifier)
	case "linux":
		if allowedcmd.IsNixOS() {
			return getBinDirOnNixOS()
		}
		return fmt.Sprintf("/usr/local/%s", identifier)
	case "windows":
		return fmt.Sprintf("C:\\Program Files\\Kolide\\Launcher-%s\\bin", identifier)
	}

	return ""
}

// getBinDirOnNixOS returns the most likely binary directory for NixOS,
// looking through the nix store to find it.
func getBinDirOnNixOS() string {
	// The binary directory on NixOS will look like, e.g.,:
	// `/nix/store/w8k7h0s9v64gqj60as756gg81c788zkp-kolide-launcher-1.4.4-6-g33c6fd9/bin`
	matches, err := filepath.Glob("/nix/store/*-kolide-launcher-*/bin")
	if err != nil || len(matches) == 0 {
		return ""
	}

	if len(matches) == 1 {
		return matches[0]
	}

	// In this case, launcher has been installed multiple times. We will pick
	// the most recent installation.
	mostRecentDir := ""
	mostRecentDirTimestamp := int64(0)
	for _, m := range matches {
		dirInfo, err := os.Stat(m)
		if err != nil {
			continue
		}

		if dirInfo.ModTime().Unix() > mostRecentDirTimestamp {
			mostRecentDir = m
			mostRecentDirTimestamp = dirInfo.ModTime().Unix()
		}
	}

	return mostRecentDir
}
