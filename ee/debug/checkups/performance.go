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

	childStats, err := performance.CurrentProcessChildStats(ctx)
	if err != nil {
		p.status = Erroring
		return fmt.Errorf("gathering child performance stats: %w", err)
	}

	// Compute checkup status.

	// We don't have access to runtime stats for the child processes, so we only check against the launcher process here.
	launcherMemOver := stats.MemInfo.GoMemUsage > golangMemUsageThreshold || stats.MemInfo.NonGoMemUsage > nonGolangMemUsageThreshold

	// We have access to CPU percent for the children, so sum those up too -- they are not accounted
	// for in the launcher process `stats.CPUPercent` automatically.
	totalCpu := stats.CPUPercent
	for _, c := range childStats {
		totalCpu += c.CPUPercent
	}
	cpuOver := totalCpu > cpuPercentThreshold

	// Make sure we have the expected number of child processes
	childProcessesMissing := len(childStats) < 2

	// Set checkup status based on launcher performance stats, plus launcher child process stats.
	if cpuOver || launcherMemOver || childProcessesMissing {
		p.status = Failing
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
	p.data["child_stats"] = childStats

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
