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

type Platform struct {
}

func (c *Platform) Name() string {
	return "Platform"
}

func (c *Platform) Run(_ context.Context, extraWriter io.Writer) error {
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

func (c *Platform) ExtraFileName() string {
	return "files.zip"
}

func (c *Platform) Status() Status {
	return Informational
}

func (c *Platform) Summary() string {
	return fmt.Sprintf("platform: %s, architecture: %s", runtime.GOOS, runtime.GOARCH)
}

func (c *Platform) Data() any {
	return map[string]any{
		"platform":     runtime.GOOS,
		"architecture": runtime.GOARCH,
	}
}
