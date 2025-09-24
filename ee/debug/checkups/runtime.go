package checkups

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/desktop/runner"
	"github.com/kolide/launcher/ee/desktop/user/client"
	"github.com/kolide/launcher/ee/desktop/user/server"
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
	// Smart discovery: try runner instance first, then system-wide discovery
	sockets, err := c.discoverDesktopSockets()
	if err != nil {
		return fmt.Errorf("discovering desktop sockets: %w", err)
	}

	if len(sockets) == 0 {
		// No desktop processes found or none support profiling, this is not an error
		return nil
	}

	// Gather profiles from each desktop process
	for i, socketInfo := range sockets {
		if err := gatherDesktopProfilesFromSocket(ctx, z, socketInfo.socketPath, socketInfo.authToken, i); err != nil {
			// Log the error but continue with other processes
			fmt.Printf("Error gathering profiles from desktop socket %s (%s): %v\n",
				socketInfo.socketPath, socketInfo.source, err)
		}
	}

	return nil
}

type desktopSocketInfo struct {
	socketPath string
	authToken  string
	source     string // "runner_instance" or "system_discovery"
}

func (c *runtimeCheckup) discoverDesktopSockets() ([]desktopSocketInfo, error) {
	// Try runner instance first (when flare is run by managing launcher)
	if processes := runner.InstanceDesktopProcessRecords(); len(processes) > 0 {
		authToken := runner.InstanceDesktopAuthToken()
		var sockets []desktopSocketInfo
		for _, processRecord := range processes {
			if socketPath := processRecord.SocketPath(); socketPath != "" {
				sockets = append(sockets, desktopSocketInfo{
					socketPath: socketPath,
					authToken:  authToken,
					source:     "runner_instance",
				})
			}
		}
		return sockets, nil
	}

	// Fall back to system-wide discovery (when flare runs standalone)
	return c.discoverDesktopSocketsSystemWide()
}

func (c *runtimeCheckup) discoverDesktopSocketsSystemWide() ([]desktopSocketInfo, error) {
	processes, err := c.findDesktopProcessesWithLsof()
	if err != nil {
		// System-wide discovery not available (e.g., on Windows)
		return nil, nil
	}

	var validSockets []desktopSocketInfo
	for _, proc := range processes {
		// Test if this socket supports profiling endpoints
		if c.testDesktopProfilingSupport(proc.socketPath, proc.authToken) {
			validSockets = append(validSockets, desktopSocketInfo{
				socketPath: proc.socketPath,
				authToken:  proc.authToken,
				source:     "system_discovery",
			})
		}
	}

	return validSockets, nil
}

type desktopProcessInfo struct {
	socketPath string
	authToken  string
}

func (c *runtimeCheckup) testDesktopProfilingSupport(socketPath, authToken string) bool {
	// Now test if profiling endpoints exist by making a quick HTTP request
	// We'll check if the endpoint exists without actually generating a profile
	testClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 5 * time.Second,
	}

	// Test cpuprofile endpoint with OPTIONS to see if it exists
	req, err := http.NewRequestWithContext(context.Background(), "OPTIONS", "http://unix/cpuprofile", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))

	resp, err := testClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// If we get anything other than 404, the profiling endpoints likely exist
	// 404 = endpoint doesn't exist, others suggest profiling support exists
	return resp.StatusCode != http.StatusNotFound
}

func gatherDesktopProfilesFromSocket(ctx context.Context, z *zip.Writer, socketPath, authToken string, processIndex int) error {
	// Create a client to communicate with the desktop server
	c := client.New(authToken, socketPath, client.WithTimeout(30*time.Second))

	// Try to ping first to see if the server is responsive
	if err := c.Ping(); err != nil {
		return fmt.Errorf("desktop server not responsive: %w", err)
	}

	// Request CPU profile
	cpuProfilePath, err := requestDesktopProfile(socketPath, authToken, "cpuprofile")
	if err != nil {
		return fmt.Errorf("requesting CPU profile: %w", err)
	}
	defer os.Remove(cpuProfilePath) // Clean up temp file

	// Request memory profile
	memProfilePath, err := requestDesktopProfile(socketPath, authToken, "memprofile")
	if err != nil {
		return fmt.Errorf("requesting memory profile: %w", err)
	}
	defer os.Remove(memProfilePath) // Clean up temp file

	// Add CPU profile to zip
	if err := addFileToZipFromPath(z, cpuProfilePath, fmt.Sprintf("desktop_%d_cpuprofile", processIndex)); err != nil {
		return fmt.Errorf("adding desktop CPU profile to zip: %w", err)
	}

	// Add memory profile to zip
	if err := addFileToZipFromPath(z, memProfilePath, fmt.Sprintf("desktop_%d_memprofile", processIndex)); err != nil {
		return fmt.Errorf("adding desktop memory profile to zip: %w", err)
	}

	return nil
}

func requestDesktopProfile(socketPath, authToken, profileType string) (string, error) {
	// Create HTTP client configured for Unix socket communication
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 30 * time.Second,
	}

	// Make POST request to profile endpoint
	url := fmt.Sprintf("http://unix/%s", profileType)
	req, err := http.NewRequestWithContext(context.Background(), "POST", url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	// Parse response
	var profileResp server.ProfileResponse
	if err := json.NewDecoder(resp.Body).Decode(&profileResp); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if profileResp.Error != "" {
		return "", fmt.Errorf("server error: %s", profileResp.Error)
	}

	if profileResp.FilePath == "" {
		return "", errors.New("server did not return file path")
	}

	return profileResp.FilePath, nil
}

func addFileToZipFromPath(z *zip.Writer, filePath, zipEntryName string) error {
	// Open the source file
	srcFile, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening source file: %w", err)
	}
	defer srcFile.Close()

	// Create entry in zip
	dst, err := z.Create(zipEntryName)
	if err != nil {
		return fmt.Errorf("creating zip entry: %w", err)
	}

	// Copy file contents to zip
	if _, err := io.Copy(dst, srcFile); err != nil {
		return fmt.Errorf("copying file to zip: %w", err)
	}

	return nil
}
