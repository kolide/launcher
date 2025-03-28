package checkups

import (
	"context"
	"encoding/json"
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

func (p *perfCheckup) Run(ctx context.Context, flareWriter io.Writer) error {
	p.data = make(map[string]any)
	stats, err := performance.CurrentProcessStats(ctx)
	if err != nil {
		return fmt.Errorf("gathering performance stats: %w", err)
	}

	p.summary = fmt.Sprintf(
		"process %d is using %.2f%% CPU,%d VMS and %d RSS (%.2f%% memory)",
		stats.Pid,
		stats.CPUPercent,
		stats.MemInfo.VMS,
		stats.MemInfo.RSS,
		stats.MemInfo.MemPercent,
	)

	p.data["stats"] = stats

	if flareWriter == io.Discard {
		return nil
	}

	jsonWriter := json.NewEncoder(flareWriter)
	return jsonWriter.Encode(stats)
}

func (p *perfCheckup) ExtraFileName() string {
	return "performance.json"
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
