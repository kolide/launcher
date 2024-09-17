//go:build !linux
// +build !linux

package checkups

import (
	"context"
	"io"
)

type coredumpCheckup struct {
}

func (c *coredumpCheckup) Name() string {
	return ""
}

func (c *coredumpCheckup) ExtraFileName() string {
	return ""
}

func (c *coredumpCheckup) Run(_ context.Context, _ io.Writer) error {
	return nil
}

func (c *coredumpCheckup) Status() Status {
	return Informational
}

func (c *coredumpCheckup) Summary() string {
	return ""
}

func (c *coredumpCheckup) Data() any {
	return nil
}
