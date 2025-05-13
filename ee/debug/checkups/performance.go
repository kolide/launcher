package checkups

import (
	"context"
	"fmt"
	"io"

	"github.com/kolide/launcher/ee/performance"
)

const (
	cpuPercentThreshold        = 30
	golangMemUsageThreshold    = 200 * 1024 * 1024 // 200 MB in bytes
	nonGolangMemUsageThreshold = 100 * 1024 * 1024 // 100 MB in bytes
)

type perfCheckup struct {
	data    map[string]any
	summary string
	status  Status
}

func (p *perfCheckup) Name() string {
	return "Performance"
}

func (p *perfCheckup) Run(ctx context.Context, _ io.Writer) error {
	p.data = make(map[string]any)
	stats, err := performance.CurrentProcessStats(ctx)
	if err != nil {
		p.status = Erroring
		return fmt.Errorf("gathering performance stats: %w", err)
	}

	memOver := stats.MemInfo.GoMemUsage > golangMemUsageThreshold || stats.MemInfo.NonGoMemUsage > nonGolangMemUsageThreshold
	cpuOver := stats.CPUPercent > cpuPercentThreshold
	if cpuOver && memOver {
		p.status = Failing
	} else if cpuOver || memOver {
		p.status = Warning
	} else {
		p.status = Passing
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
	return p.status
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
