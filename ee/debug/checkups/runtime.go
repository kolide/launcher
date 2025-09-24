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
	"github.com/kolide/launcher/ee/desktop/user/client"
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

	if err := pprof.StartCPUProfile(out); err != nil {
		return fmt.Errorf("starting CPU profile: %w", err)
	}

	// Use a channel to wait for the profiling to complete
	done := make(chan struct{})

	// Move the sleep and StopCPUProfile into a goroutine
	go func() {
		// cpu profile is really meant to run over a period of time, capturing background information
		time.Sleep(5 * time.Second)
		pprof.StopCPUProfile()
		close(done)
	}()

	// Wait for profiling to complete with a timeout
	select {
	case <-done:
		return nil
	case <-time.After(10 * time.Second):
		return errors.New("timeout waiting for CPU profile to complete")
	}
}

func (c *runtimeCheckup) gatherDesktopProfiles(ctx context.Context, z *zip.Writer) error {
	// Get desktop process records from knapsack
	processRecords := c.k.GetDesktopProcessRecords()
	if len(processRecords) == 0 {
		// No desktop processes running, this is not an error
		return nil
	}

	authToken := c.k.GetDesktopAuthToken()
	if authToken == "" {
		// No auth token available, cannot profile desktop processes
		return nil
	}

	// Gather profiles from each desktop process
	for i, processRecord := range processRecords {
		socketPath := processRecord.SocketPath()
		if socketPath == "" {
			continue // Skip processes without socket paths
		}

		if err := gatherDesktopProfilesFromSocket(ctx, z, socketPath, authToken, i); err != nil {
			// Log the error but continue with other processes
			fmt.Printf("Error gathering profiles from desktop process %d (socket %s): %v\n",
				processRecord.Pid(), socketPath, err)
		}
	}

	return nil
}

func gatherDesktopProfilesFromSocket(ctx context.Context, z *zip.Writer, socketPath, authToken string, processIndex int) error {
	// Create desktop client for profile requests
	desktopClient := client.New(authToken, socketPath, client.WithTimeout(30*time.Second))

	// Request CPU profile
	cpuProfilePath, err := desktopClient.RequestProfile(ctx, "cpuprofile")
	if err != nil {
		return fmt.Errorf("requesting CPU profile: %w", err)
	}
	defer os.Remove(cpuProfilePath) // Clean up temp file

	// Request memory profile
	memProfilePath, err := desktopClient.RequestProfile(ctx, "memprofile")
	if err != nil {
		return fmt.Errorf("requesting memory profile: %w", err)
	}
	defer os.Remove(memProfilePath) // Clean up temp file

	// Add CPU profile to zip
	if err := addFileToZip(z, cpuProfilePath, fmt.Sprintf("desktop_%d_cpuprofile", processIndex)); err != nil {
		return fmt.Errorf("adding desktop CPU profile to zip: %w", err)
	}

	// Add memory profile to zip
	if err := addFileToZip(z, memProfilePath, fmt.Sprintf("desktop_%d_memprofile", processIndex)); err != nil {
		return fmt.Errorf("adding desktop memory profile to zip: %w", err)
	}

	return nil
}
