package performance

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/kolide/launcher/ee/observability"
	"github.com/shirou/gopsutil/v4/process"
)

/*
MemInfo is a helper struct with memory stats derived from both:
- process.MemoryInfoStat (e.g. RSS, VMS). these help to give a picture of the usage from the OS perspective
- runtime.MemStats (e.g. heap stats). these help to give a picture of the go runtime usage
Used together, we are able to estimate things like allocations outside of our golang runtime (see nonGoMemUsage).
The runtime MemStats can be confusing - here is an attempt at outlining some of the fields we're interested in:

	Sys is total bytes of memory obtained from the OS. The virtual address space reserved by the Go runtime for:
	Sys                             = HeapSys + StackSys + MSpanSys + MCacheSys + BuckHashSys + GCSys + OtherSys
		HeapSys                     = HeapInUse + HeapIdle. estimates the largest size the heap has had
			HeapInUse               = HeapAlloc + currently unused memory that has been dedicated to particular size classes
				HeapAlloc           = bytes allocated for heap objects currently inuse + unreachable and pending GC
			HeapIdle                = HeapReleased + bytes that are unused but have not yet been returned to OS
				HeapReleased        = bytes of physical memory returned to the OS
				...
		...

see https://github.com/golang/go/issues/32284#issuecomment-496967090 and https://pkg.go.dev/runtime#MemStats for further reading
*/
type MemInfo struct {
	RSS           uint64  `json:"rss_bytes"`        // bytes - memory allocated to the process held in RAM. includes stack and heap memory
	VMS           uint64  `json:"vms_bytes"`        // bytes - all memory the process can access, including allocated+unused, and from shared libraries
	HeapTotal     uint64  `json:"heap_total_bytes"` // bytes - see heapTotal for details
	GoMemUsage    uint64  `json:"go_mem_bytes"`     // bytes - see goMemUsage for details
	NonGoMemUsage uint64  `json:"non_go_mem_bytes"` // bytes - see nonGoMemUsage for details
	MemPercent    float32 `json:"mem_percent"`      // percent of memory in use (RSS) vs available on machine
}

type IO struct {
	ReadCount  uint64 `json:"read_count"`
	WriteCount uint64 `json:"write_count"`
	ReadBytes  uint64 `json:"read_bytes"`
	WriteBytes uint64 `json:"write_bytes"`
}

type PerformanceStats struct {
	Pid        int      `json:"pid"`
	Exe        string   `json:"exe"`
	Cmdline    string   `json:"cmdline"`
	MemInfo    *MemInfo `json:"mem_info"`
	IO         *IO      `json:"io,omitempty"` // Not available everywhere
	CPUPercent float64  `json:"cpu_percent"`
}

// CurrentProcessStats gets memory and CPU stats for the current process;
// it has the side effect of recording those metrics. (Pretty much any time we
// do the work of collecting these stats, we want to emit measurements to our
// metrics instruments.)
func CurrentProcessStats(ctx context.Context) (*PerformanceStats, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	pid := os.Getpid()
	return ProcessStatsForPid(ctx, pid)
}

func ProcessStatsForPid(ctx context.Context, pid int) (*PerformanceStats, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	proc, err := process.NewProcessWithContext(ctx, int32(pid))
	if err != nil {
		return nil, fmt.Errorf("getting process handle for pid %d: %w", pid, err)
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	ps, memInfo, err := statsForProcess(ctx, proc)
	if err != nil {
		return nil, fmt.Errorf("gathering stats for process: %w", err)
	}

	ps.MemInfo.NonGoMemUsage = nonGoMemUsage(memInfo, &memStats)
	ps.MemInfo.GoMemUsage = goMemUsage(&memStats)
	ps.MemInfo.HeapTotal = heapTotal(&memStats)

	// Record stats
	observability.GoMemoryUsageGauge.Record(ctx, int64(ps.MemInfo.GoMemUsage))
	observability.NonGoMemoryUsageGauge.Record(ctx, int64(ps.MemInfo.NonGoMemUsage))
	observability.MemoryPercentGauge.Record(ctx, int64(ps.MemInfo.MemPercent))
	observability.CpuPercentGauge.Record(ctx, int64(ps.CPUPercent))
	observability.RSSHistogram.Record(ctx, int64(ps.MemInfo.RSS))
	if ps.IO != nil {
		observability.IOReadsCounter.Add(ctx, int64(ps.IO.ReadCount))
		observability.IOWritesCounter.Add(ctx, int64(ps.IO.WriteCount))
	}

	return ps, nil
}

// CurrentProcessChildStats gets memory and CPU stats for the current process's child processes;
// it has the side effect of recording those metrics. (Pretty much any time we
// do the work of collecting these stats, we want to emit measurements to our
// metrics instruments.)
func CurrentProcessChildStats(ctx context.Context) ([]*PerformanceStats, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	pid := os.Getpid()
	return ChildProcessStatsForPid(ctx, int32(pid))
}

func ChildProcessStatsForPid(ctx context.Context, pid int32) ([]*PerformanceStats, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	proc, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		return nil, fmt.Errorf("getting process handle for pid %d: %w", pid, err)
	}

	childProcesses, err := proc.ChildrenWithContext(ctx)
	// ChildrenWithContext uses pgrep, which will exit with exit status 1 if there were no matching processes
	// (i.e. no child processes). This is unexpected for us -- launcher should typically have child processes --
	// but it's not necessarily an error. Only return an error if we got an actual error string back here;
	// callers can handle an empty list of children appropriately in the case that we received exit status 1.
	if err != nil && !strings.Contains(err.Error(), "exit status 1") {
		return nil, fmt.Errorf("getting child processes for pid %d: %w", pid, err)
	}

	stats := make([]*PerformanceStats, 0)
	for _, childProcess := range childProcesses {
		ps, _, err := statsForProcess(ctx, childProcess)
		if err != nil {
			continue
		}
		stats = append(stats, ps)

		// Record osquery metrics
		if strings.Contains(ps.Cmdline, "osquery") {
			observability.OsqueryRssHistogram.Record(ctx, int64(ps.MemInfo.RSS))
			observability.OsqueryCpuPercentHistogram.Record(ctx, ps.CPUPercent)
		}

		// Record launcher desktop metrics. We check the executable for "launcher" string, rather than
		// cmdline, to ignore the sudo parent desktop process on macOS. We may need to look in the
		// grandchild processes instead.
		if strings.Contains(ps.Exe, "launcher") && strings.HasSuffix(ps.Cmdline, "desktop") {
			observability.DesktopRssHistogram.Record(ctx, int64(ps.MemInfo.RSS))
			observability.DesktopCpuPercentHistogram.Record(ctx, ps.CPUPercent)
		}

		// We want to grab one more level of child processes, to account for the desktop process
		// being invoked with sudo first on macOS.
		grandchildProcesses, err := childProcess.ChildrenWithContext(ctx)
		if err != nil {
			continue
		}
		for _, grandchildProcess := range grandchildProcesses {
			ps, _, err := statsForProcess(ctx, grandchildProcess)
			if err != nil {
				continue
			}
			stats = append(stats, ps)

			// Record launcher desktop metrics. We check the executable for "launcher" string, rather than
			// cmdline, to ignore the sudo parent desktop process on macOS.
			if strings.Contains(ps.Exe, "launcher") && strings.HasSuffix(ps.Cmdline, "desktop") {
				observability.DesktopRssHistogram.Record(ctx, int64(ps.MemInfo.RSS))
				observability.DesktopCpuPercentHistogram.Record(ctx, ps.CPUPercent)
			}
		}
	}

	return stats, nil
}

func statsForProcess(ctx context.Context, proc *process.Process) (*PerformanceStats, *process.MemoryInfoStat, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	ps := &PerformanceStats{
		Pid:     int(proc.Pid),
		MemInfo: &MemInfo{},
	}

	if exe, err := proc.ExeWithContext(ctx); err != nil {
		return nil, nil, fmt.Errorf("gathering exe: %w", err)
	} else {
		ps.Exe = exe
	}

	if cmdline, err := proc.CmdlineWithContext(ctx); err != nil {
		return nil, nil, fmt.Errorf("gathering cmdline: %w", err)
	} else {
		ps.Cmdline = cmdline
	}

	memInfo, err := proc.MemoryInfoWithContext(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("gathering mem info: %w", err)
	}
	ps.MemInfo.RSS = memInfo.RSS
	ps.MemInfo.VMS = memInfo.VMS

	if memPercent, err := proc.MemoryPercentWithContext(ctx); err != nil {
		return nil, nil, fmt.Errorf("gathering mem percent: %w", err)
	} else {
		ps.MemInfo.MemPercent = memPercent
	}

	if cpuPercent, err := proc.CPUPercentWithContext(ctx); err != nil {
		return nil, nil, fmt.Errorf("gathering cpu percent: %w", err)
	} else {
		ps.CPUPercent = cpuPercent
	}

	if ioCounters, err := proc.IOCountersWithContext(ctx); err != nil {
		// We know this is only available on Linux/Windows, but not Darwin --
		// so we suppress the error here for darwin.
		if runtime.GOOS != "darwin" {
			return nil, nil, fmt.Errorf("gathering I/O counters: %w", err)
		}
	} else {
		ps.IO = &IO{
			ReadCount:  ioCounters.ReadCount,
			WriteCount: ioCounters.WriteCount,
			ReadBytes:  ioCounters.ReadBytes,
			WriteBytes: ioCounters.WriteBytes,
		}
	}

	return ps, memInfo, nil
}

func heapTotal(ms *runtime.MemStats) uint64 {
	freeBytes := ms.HeapIdle - ms.HeapReleased
	return ms.HeapInuse + freeBytes
}

// This is what the go runtime is responsible for, when attempting to enforce a soft memory limit
// it attempts to maintain Sys - HeapReleased. see https://pkg.go.dev/runtime/debug#SetMemoryLimit
// return value is in bytes
func goMemUsage(ms *runtime.MemStats) uint64 {
	return ms.Sys - ms.HeapReleased
}

// nonGoMemUsage is looking at all inuse memory allocated from the OS perspective minus the go memory
// accounted for by go's runtime. this can indicate outside usage (e.g. CGO or other external allocations)
func nonGoMemUsage(memInfo *process.MemoryInfoStat, ms *runtime.MemStats) uint64 {
	return memInfo.RSS - goMemUsage(ms)
}
