//go:build !linux
// +build !linux

package checkups

import (
	"context"
	"io"
)

type gnomeExtensions struct {
}

func (c *gnomeExtensions) Name() string {
	return ""
}

func (c *gnomeExtensions) ExtraFileName() string {
	return ""
}

func (c *gnomeExtensions) Run(_ context.Context, _ io.Writer) error {
	return nil
}

func (c *gnomeExtensions) Status() Status {
	return Informational
}

func (c *gnomeExtensions) Summary() string {
	return ""
}

func (c *gnomeExtensions) Data() any {
	return nil
}
