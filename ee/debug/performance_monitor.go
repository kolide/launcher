package debug

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/debug/checkups"
	"github.com/kolide/launcher/ee/debug/shipper"
	"github.com/kolide/launcher/ee/performance"
)

const (
	performanceCheckInitialDelay      = 5 * time.Minute
	performanceCheckInterval          = 15 * time.Minute
	golangMemUsageThresholdForFlare   = 1000 * 1024 * 1024 // 1 GB in bytes
	cpuUsageThresholdForFlare         = 10
	minimumFlareResendIntervalSeconds = 24 * 60 * 60 // 1 day
	flareUploadRequestUrl             = "https://api.kolide.com/api/agent/flare"
	notePrefix                        = "automated flare"
)

type performanceMonitor struct {
	knapsack      types.Knapsack
	slogger       *slog.Logger
	lastFlareSent *atomic.Int64
	interrupt     chan struct{}
	interrupted   *atomic.Bool
}

func NewPerformanceMonitor(k types.Knapsack) *performanceMonitor {
	return &performanceMonitor{
		knapsack:      k,
		slogger:       k.Slogger().With("component", "performance_monitor"),
		lastFlareSent: &atomic.Int64{},
		interrupt:     make(chan struct{}),
		interrupted:   &atomic.Bool{},
	}
}

func (p *performanceMonitor) Execute() error {
	// Wait a bit before beginning monitoring
	select {
	case <-p.interrupt:
		return nil
	case <-time.After(performanceCheckInitialDelay):
		break
	}

	ticker := time.NewTicker(performanceCheckInterval)
	defer ticker.Stop()
	for {
		p.checkPerformance()

		select {
		case <-ticker.C:
			continue
		case <-p.interrupt:
			return nil
		}
	}
}

// checkPerformance gathers stats for the current process and assesses them. If performance
// seems egregiously bad, it triggers a flare upload.
func (p *performanceMonitor) checkPerformance() {
	// Only gather stats if performance monitoring is enabled
	if !p.knapsack.PerformanceMonitoringEnabled() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	stats, err := performance.CurrentProcessStats(ctx)
	if err != nil {
		p.slogger.Log(ctx, slog.LevelWarn,
			"could not get current process stats",
			"err", err,
		)
		return
	}

	doUpload, uploadReason := p.shouldTriggerFlareUpload(ctx, stats)
	if !doUpload {
		return
	}

	// If we've noticed extraordinarily high Golang memory or CPU usage, trigger a flare.
	// Since we're looking at Golang memory and CPU here, the flare's memprofile and cpuprofile
	// will be useful in understanding what's using memory.
	p.slogger.Log(ctx, slog.LevelWarn,
		"noticed abnormally high Golang memory and/or CPU usage while monitoring, triggering flare",
		"golang_memory_usage", stats.MemInfo.GoMemUsage,
		"non_golang_memory_usage", stats.MemInfo.NonGoMemUsage,
		"rss", stats.MemInfo.RSS,
		"cpu", stats.CPUPercent,
	)

	flareShipper, err := shipper.New(p.knapsack, shipper.WithUploadRequestURL(flareUploadRequestUrl), shipper.WithNote(p.uploadNote(uploadReason)))
	if err != nil {
		p.slogger.Log(ctx, slog.LevelError,
			"could not create flare shipper to capture high Golang memory and/or CPU usage",
			"err", err,
		)
		return
	}

	if err := checkups.RunFlare(ctx, p.knapsack, flareShipper, checkups.InSituPerformanceEnvironment); err != nil {
		p.slogger.Log(ctx, slog.LevelError,
			"could not run and ship flare to capture high Golang memory and/or CPU usage",
			"err", err,
		)
		return
	}

	p.slogger.Log(ctx, slog.LevelInfo,
		"successfully triggered flare to capture abnormally high Golang memory and/or CPU usage",
		"flare_id", flareShipper.Name(),
	)

	p.lastFlareSent.Store(time.Now().Unix())
}

// shouldTriggerFlareUpload checks stats and last sent flare timestamp to determine if it should
// trigger an automatic flare upload. If a flare should be uploaded, it also returns the reason
// for the flare, to include in the upload note.
func (p *performanceMonitor) shouldTriggerFlareUpload(ctx context.Context, stats *performance.PerformanceStats) (bool, string) {
	// Check if performance stats warrant a flare upload
	excessiveMemUsage := stats.MemInfo.GoMemUsage >= golangMemUsageThresholdForFlare
	excessiveCpuUsage := stats.CPUPercent >= cpuUsageThresholdForFlare
	if !excessiveMemUsage && !excessiveCpuUsage {
		return false, ""
	}

	// Make sure we haven't sent a flare too recently
	nextAllowableFlareTimestamp := p.lastFlareSent.Load() + minimumFlareResendIntervalSeconds
	if nextAllowableFlareTimestamp > time.Now().Unix() {
		p.slogger.Log(ctx, slog.LevelWarn,
			"noticed abnormally high Golang memory and/or CPU usage while monitoring, but triggered flare too recently",
			"golang_memory_usage", stats.MemInfo.GoMemUsage,
			"non_golang_memory_usage", stats.MemInfo.NonGoMemUsage,
			"rss", stats.MemInfo.RSS,
			"cpu", stats.CPUPercent,
		)
		return false, ""
	}

	uploadReason := "high Golang memory usage"
	if excessiveMemUsage && excessiveCpuUsage {
		uploadReason = "high Golang memory and CPU usage"
	} else if excessiveCpuUsage {
		uploadReason = "high CPU usage"
	}

	return true, uploadReason
}

// uploadNote creates a flare note, identifying that this is a) an automated flare, b) the reason
// the flare is being created, and c) the device via the serial, if available.
func (p *performanceMonitor) uploadNote(reason string) string {
	deviceDetails := p.knapsack.GetEnrollmentDetails()
	if deviceDetails.HardwareSerial != "" {
		return fmt.Sprintf("%s: %s (%s)", notePrefix, reason, deviceDetails.HardwareSerial)
	}
	return fmt.Sprintf("%s: %s (unknown serial)", notePrefix, reason)
}

func (p *performanceMonitor) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if p.interrupted.Swap(true) {
		return
	}

	p.interrupt <- struct{}{}
}
