//go:build !darwin
// +build !darwin

package checkups

import (
	"context"
	"io"
)

type launchdCheckup struct {
}

func (c *launchdCheckup) Name() string {
	return ""
}

func (c *launchdCheckup) Run(_ context.Context, _ io.Writer) error {
	return nil
}

func (c *launchdCheckup) ExtraFileName() string {
	return ""
}

func (c *launchdCheckup) Status() Status {
	return Informational
}

func (c *launchdCheckup) Summary() string {
	return ""
}

func (c *launchdCheckup) Data() any {
	return nil
}
