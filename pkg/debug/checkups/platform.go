package checkups

import (
	"context"
	"fmt"
	"io"
	"runtime"
)

type Platform struct {
}

func (c *Platform) Name() string {
	return "Platform"
}

func (c *Platform) Run(_ context.Context, _ io.Writer) error {
	return nil
}

func (c *Platform) ExtraFileName() string {
	return ""
}

func (c *Platform) Status() Status {
	return Informational
}

func (c *Platform) Summary() string {
	return fmt.Sprintf("platform: %s, architecture: %s", runtime.GOOS, runtime.GOARCH)
}

func (c *Platform) Data() any {
	return map[string]string{
		"platform":     runtime.GOOS,
		"architecture": runtime.GOARCH,
	}
}
