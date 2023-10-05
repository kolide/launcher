package checkups

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"runtime"
)

var potentialFiles = []string{
	"/etc/os-release",
	"/etc/redhat-release",
	"/etc/gentoo-release",
	"/etc/issue",
	"/etc/lsb-release",
}

type platform struct{}

func (c *platform) Name() string {
	return "Platform"
}

func (c *platform) Run(_ context.Context, extraWriter io.Writer) error {
	if extraWriter == io.Discard {
		return nil
	}

	z := zip.NewWriter(extraWriter)
	defer z.Close()

	for _, f := range potentialFiles {
		if err := addFileToZip(z, f); err != nil {
			return fmt.Errorf("adding %s to zip: %w", f, err)
		}
	}

	return nil
}

func (c *platform) ExtraFileName() string {
	return "files.zip"
}

func (c *platform) Status() Status {
	return Informational
}

func (c *platform) Summary() string {
	return fmt.Sprintf("platform: %s, architecture: %s", runtime.GOOS, runtime.GOARCH)
}

func (c *platform) Data() map[string]any {
	return map[string]any{
		"platform":     runtime.GOOS,
		"architecture": runtime.GOARCH,
	}
}
