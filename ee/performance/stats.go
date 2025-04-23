package performance

import (
	"context"
	"fmt"
	"os"
	"runtime"

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

type PerformanceStats struct {
	Pid        int      `json:"pid"`
	Exe        string   `json:"exe"`
	MemInfo    *MemInfo `json:"mem_info"`
	CPUPercent float64  `json:"cpu_percent"`
}

func CurrentProcessStats(ctx context.Context) (*PerformanceStats, error) {
	pid := os.Getpid()
	return ProcessStatsForPid(ctx, pid)
}

func ProcessStatsForPid(ctx context.Context, pid int) (*PerformanceStats, error) {
	proc, err := process.NewProcessWithContext(ctx, int32(pid))
	if err != nil {
		return nil, fmt.Errorf("getting process handle for pid %d: %w", pid, err)
	}

	ps := &PerformanceStats{
		Pid:     pid,
		MemInfo: &MemInfo{},
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	if exe, err := proc.ExeWithContext(ctx); err != nil {
		return nil, fmt.Errorf("gathering exe: %w", err)
	} else {
		ps.Exe = exe
	}

	if memInfo, err := proc.MemoryInfoWithContext(ctx); err != nil {
		return nil, fmt.Errorf("gathering mem info: %w", err)
	} else {
		ps.MemInfo.RSS = memInfo.RSS
		ps.MemInfo.VMS = memInfo.VMS
		ps.MemInfo.NonGoMemUsage = nonGoMemUsage(memInfo, &memStats)
	}

	if memPercent, err := proc.MemoryPercentWithContext(ctx); err != nil {
		return nil, fmt.Errorf("gathering mem percent: %w", err)
	} else {
		ps.MemInfo.MemPercent = memPercent
	}

	if cpuPercent, err := proc.CPUPercentWithContext(ctx); err != nil {
		return nil, fmt.Errorf("gathering cpu percent: %w", err)
	} else {
		ps.CPUPercent = cpuPercent
	}

	ps.MemInfo.GoMemUsage = goMemUsage(&memStats)
	ps.MemInfo.HeapTotal = heapTotal(&memStats)

	return ps, nil
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
