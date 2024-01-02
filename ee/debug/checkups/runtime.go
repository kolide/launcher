package checkups

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"runtime/pprof"
	"time"
)

type runtimeCheckup struct {
}

func (c *runtimeCheckup) Name() string {
	return "Runtime"
}

func (c *runtimeCheckup) Run(ctx context.Context, extraWriter io.Writer) error {
	extraZip := zip.NewWriter(extraWriter)
	defer extraZip.Close()

	if err := gatherMemStats(extraZip); err != nil {
		return fmt.Errorf("gathering mem stats: %w", err)
	}

	if err := gatherStack(extraZip); err != nil {
		return fmt.Errorf("gathering stack: %w", err)
	}

	if err := gatherPprofMemory(extraZip); err != nil {
		return fmt.Errorf("gathering memory profile: %w", err)
	}

	if err := gatherPprofCpu(extraZip); err != nil {
		return fmt.Errorf("gathering cpu profile: %w", err)
	}

	return nil

}

func (c *runtimeCheckup) ExtraFileName() string {
	return "runtime.zip"
}

func (c *runtimeCheckup) Status() Status {
	return Informational
}

func (c *runtimeCheckup) Summary() string {
	return "N/A"
}

func (c *runtimeCheckup) Data() any {
	return nil
}

func gatherMemStats(z *zip.Writer) error {
	out, err := z.Create("memstats.json")
	if err != nil {
		return fmt.Errorf("creating memstats.json: %w", err)
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	if err := json.NewEncoder(out).Encode(m); err != nil {
		return fmt.Errorf("json encode: %w", err)
	}

	return nil
}

func gatherStack(z *zip.Writer) error {
	out, err := z.Create("stack")
	if err != nil {
		return fmt.Errorf("creating stack: %w", err)
	}

	buf := make([]byte, 1<<16)
	stacklen := runtime.Stack(buf, true)
	if _, err := out.Write(buf[0:stacklen]); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}

func gatherPprofMemory(z *zip.Writer) error {
	out, err := z.Create("memprofile")
	if err != nil {
		return fmt.Errorf("creating memprofile: %w", err)
	}
	runtime.GC() // get up-to-date statistics
	if err := pprof.WriteHeapProfile(out); err != nil {
		return fmt.Errorf("writing memory profile: %w", err)
	}

	return nil

}

func gatherPprofCpu(z *zip.Writer) error {
	out, err := z.Create("cpuprofile")
	if err != nil {
		return fmt.Errorf("creating cpuprofile: %w", err)
	}

	if err := pprof.StartCPUProfile(out); err != nil {
		return fmt.Errorf("starting CPU profile: %w", err)
	}
	defer pprof.StopCPUProfile()

	// cpu profile is really meant to run over a period of time, capturing background information. But,
	// We're not really setup for that right now. So this is a quick capture, of what's happening in background
	// threads. Better would be to have a goroutine writing this into a temp buffer somewhere.
	time.Sleep(100 * time.Millisecond)

	return nil
}
