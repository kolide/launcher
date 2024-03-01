//go:build !windows
// +build !windows

package checkups

import (
	"context"
	"io"
)

type intuneCheckup struct{}

func (i *intuneCheckup) Name() string {
	return ""
}

func (i *intuneCheckup) Run(_ context.Context, _ io.Writer) error {
	return nil
}

func (i *intuneCheckup) ExtraFileName() string {
	return ""
}

func (i *intuneCheckup) Status() Status {
	return Informational
}

func (i *intuneCheckup) Summary() string {
	return ""
}

func (i *intuneCheckup) Data() any {
	return nil
}
