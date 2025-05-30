//go:build !windows
// +build !windows

package checkups

import (
	"context"
	"io"

	"github.com/kolide/launcher/ee/agent/types"
)

type servicesCheckup struct {
	k types.Knapsack
}

func (s *servicesCheckup) Name() string {
	return ""
}

func (s *servicesCheckup) Run(ctx context.Context, extraWriter io.Writer) error {
	return nil
}

func (s *servicesCheckup) ExtraFileName() string {
	return ""
}

func (s *servicesCheckup) Status() Status {
	return Informational
}

func (s *servicesCheckup) Summary() string {
	return ""
}

func (s *servicesCheckup) Data() any {
	return nil
}
