//go:build !windows
// +build !windows

package checkups

import (
	"context"
	"io"
)

type powerCheckup struct{}

func (p *powerCheckup) Name() string {
	return ""
}

func (p *powerCheckup) Run(ctx context.Context, extraWriter io.Writer) error {
	return nil
}

func (p *powerCheckup) ExtraFileName() string {
	return ""
}

func (p *powerCheckup) Status() Status {
	return Informational
}

func (p *powerCheckup) Summary() string {
	return ""
}

func (p *powerCheckup) Data() any {
	return nil
}
