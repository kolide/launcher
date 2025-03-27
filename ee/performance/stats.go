package performance

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/shirou/gopsutil/v4/process"
)

type (
	MemInfo struct {
		RSS        uint64  // bytes
		VMS        uint64  // bytes
		MemPercent float32 // percent of memory in use (RSS) vs available on machine
		HeapTotal  uint64
		GoMemUsage uint64
	}

	PerformanceStats struct {
		Pid        int
		Exe        string
		MemInfo    *MemInfo
		CPUPercent float64
	}
)

func GetStats(ctx context.Context) (*PerformanceStats, error) {
	pid := os.Getpid()
	proc, err := process.NewProcessWithContext(ctx, int32(pid))
	if err != nil {
		return nil, fmt.Errorf("getting process handle for pid %d: %w", pid, err)
	}

	ps := &PerformanceStats{
		Pid:     pid,
		MemInfo: &MemInfo{},
	}

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

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	ps.MemInfo.GoMemUsage = memStats.Sys - memStats.HeapReleased
	ps.MemInfo.HeapTotal = heapTotal(&memStats)

	return ps, nil
}

func heapTotal(m *runtime.MemStats) uint64 {
	bytesAllocatedToObjects := m.HeapAlloc // both live and dead
	freeBytes := m.HeapIdle - m.HeapReleased
	unusedBytes := m.HeapInuse - m.HeapAlloc

	return bytesAllocatedToObjects + freeBytes + unusedBytes
}
