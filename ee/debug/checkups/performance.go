package checkups

import (
	"context"
	"fmt"
	"io"

	"github.com/kolide/launcher/ee/performance"
)

type perfCheckup struct {
	data    map[string]any
	summary string
}

func (p *perfCheckup) Name() string {
	return "Performance"
}

func (p *perfCheckup) Run(ctx context.Context, _ io.Writer) error {
	p.data = make(map[string]any)
	stats, err := performance.CurrentProcessStats(ctx)
	if err != nil {
		return fmt.Errorf("gathering performance stats: %w", err)
	}

	p.summary = fmt.Sprintf(
		"process %d is using %.2f%% CPU, RSS: %.2f MB (%.2f%% memory)",
		stats.Pid,
		stats.CPUPercent,
		bytesToMB(stats.MemInfo.RSS),
		stats.MemInfo.MemPercent,
	)

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
	return p.summary
}

func (p *perfCheckup) Data() any {
	return p.data
}

func bytesToMB(bytes uint64) float64 {
	return float64(bytes) / (1024 * 1024)
}
