package checkups

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
)

type runtimeCheckup struct {
	k types.Knapsack
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

	if err := c.gatherDesktopProfiles(ctx, extraZip); err != nil {
		return fmt.Errorf("gathering desktop profiles: %w", err)
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

	// A digression on CPU profiling in Go...
	//
	// We are not experts, and this might be wrong. But as I understand it, go will sample what the
	// CPU is doing N times per second. The default value is 100Hz, but it is very unclear if this
	// is correct.
	//
	// One theory, is that this is too low on modern CPUs, and it's easy to miss things. There is
	// a lot of discussion on this in https://github.com/golang/go/issues/40094 and
	// linked issues.
	//
	// However, there is another school of thought, that, at least on windows, it's too high. The windows
	// default timer is 15.6ms (about 64Hz) which is well below that 100Hz. See discussion in https://github.com/golang/go/issues/61665#issuecomment-1659714314
	// However that also links to a CL where go starts using the high resolution timers -- https://go-review.googlesource.com/c/go/+/514375
	//
	// Because we don't understand, we are leaving it alone for now. If we do choose to change it, we
	// can use the `runtime.SetCPUProfileRate(rate int)` function to do so.
	//
	// Also, note that changing this will cause profiling to produce a spurious
	// error -- "runtime: cannot set cpu profile rate until previous profile has finished"

	if err := pprof.StartCPUProfile(out); err != nil {
		return fmt.Errorf("starting CPU profile: %w", err)
	}

	// Use a channel to wait for the profiling to complete
	done := make(chan struct{})

	// Move the sleep and StopCPUProfile into a goroutine
	go func() {
		// cpu profile is really meant to run over a period of time, capturing background information
		time.Sleep(15 * time.Second)
		pprof.StopCPUProfile()
		close(done)
	}()

	// Wait for profiling to complete with a timeout
	select {
	case <-done:
		return nil
	case <-time.After(20 * time.Second):
		return errors.New("timeout waiting for CPU profile to complete")
	}
}

func (c *runtimeCheckup) gatherDesktopProfiles(ctx context.Context, z *zip.Writer) error {
	// Request CPU profiles from all desktop processes
	cpuProfilePaths, err := c.k.RequestProfile(ctx, "cpuprofile")
	if err != nil {
		return fmt.Errorf("requesting CPU profiles: %w", err)
	}

	// Request memory profiles from all desktop processes
	memProfilePaths, err := c.k.RequestProfile(ctx, "memprofile")
	if err != nil {
		return fmt.Errorf("requesting memory profiles: %w", err)
	}

	// Add CPU profiles to zip
	for i, profilePath := range cpuProfilePaths {
		if err := addFileToZip(z, profilePath, fmt.Sprintf("desktop_%d_cpuprofile", i)); err != nil {
			fmt.Printf("Error adding desktop CPU profile to zip: %v\n", err)
			continue
		}
		// Clean up temp file after adding to zip
		if err := os.Remove(profilePath); err != nil {
			fmt.Printf("Warning: failed to clean up temp CPU profile file %s: %v\n", profilePath, err)
		}
	}

	// Add memory profiles to zip
	for i, profilePath := range memProfilePaths {
		if err := addFileToZip(z, profilePath, fmt.Sprintf("desktop_%d_memprofile", i)); err != nil {
			fmt.Printf("Error adding desktop memory profile to zip: %v\n", err)
			continue
		}
		// Clean up temp file after adding to zip
		if err := os.Remove(profilePath); err != nil {
			fmt.Printf("Warning: failed to clean up temp memory profile file %s: %v\n", profilePath, err)
		}
	}

	return nil
}
