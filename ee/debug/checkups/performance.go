package checkups

import (
	"context"
	"fmt"
	"io"

	"github.com/kolide/launcher/ee/performance"
)

type perfCheckup struct {
	data map[string]any
}

func (p *perfCheckup) Name() string {
	return "Performance"
}

func (p *perfCheckup) Run(ctx context.Context, _ io.Writer) error {
	p.data = make(map[string]any)
	stats, err := performance.GetStats(ctx)
	if err != nil {
		return fmt.Errorf("gathering performance stats: %w", err)
	}

	p.data["stats"] = stats

	return nil
}

func (p *perfCheckup) ExtraFileName() string {
	return ""
}

func (p *perfCheckup) Status() Status {
	return Informational
}

func (p *perfCheckup) Summary() string {
	return ""
}

func (p *perfCheckup) Data() any {
	return p.data
}
