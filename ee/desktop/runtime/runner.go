package runtime

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/shirou/gopsutil/process"
)

// DesktopUsersProcessesRunner creates a launcher desktop process each time it detects
// a new console (GUI) user. If the current console user's desktop process dies, it
// will create a new one.
// Initialize with New().
type DesktopUsersProcessesRunner struct {
	logger            log.Logger
	executionInterval time.Duration
	interrupt         chan struct{}
	// uidProcs is a map of uid to desktop process
	uidProcs map[string]*os.Process
	// procsWg is a WaitGroup to wait for all desktop processes to finish during an interrupt
	procsWg *sync.WaitGroup
	// procsWgTimeout how long to wait for desktop proccesses to finish on interrupt
	procsWgTimeout time.Duration
	// executablePath is the path to the launcher executable. Currently this is only set during testing
	// due to needing to build the binary to test as a result of some test harness weirdness.
	// See runner_test.go for more details.
	executablePath string
}

// New creates and returns a new DesktopUsersProcessesRunner runner and initializes all required fields
func New(logger log.Logger, executionInterval time.Duration) *DesktopUsersProcessesRunner {
	return &DesktopUsersProcessesRunner{
		logger:            logger,
		interrupt:         make(chan struct{}),
		uidProcs:          make(map[string]*os.Process),
		executionInterval: executionInterval,
		procsWg:           &sync.WaitGroup{},
		procsWgTimeout:    time.Second * 5,
	}
}

// Execute immediately checks if the current console user has a desktop process running. If not, it will start a new one.
// Then repeats based on the executionInterval.
func (r *DesktopUsersProcessesRunner) Execute() error {
	f := func() {
		if err := r.runDesktopNative(); err != nil {
			level.Error(r.logger).Log(
				"msg", "error running desktop",
				"err", err,
			)
		}
	}

	f()

	ticker := time.NewTicker(r.executionInterval)
	for {
		select {
		case <-ticker.C:
			f()
		case <-r.interrupt:
			level.Debug(r.logger).Log("msg", "interrupt received, exiting desktop execute loop")
			return nil
		}
	}
}

// Interrupt stops creating launcher desktop processes and kills any existing ones.
func (r *DesktopUsersProcessesRunner) Interrupt(interruptError error) {
	level.Debug(r.logger).Log(
		"msg", "sending interrupt to desktop users processes runner",
		"err", interruptError,
	)

	r.interrupt <- struct{}{}

	wgDone := make(chan struct{})
	go func() {
		defer close(wgDone)
		r.procsWg.Wait()
	}()

	signal := syscall.SIGTERM
	for _, proc := range r.uidProcs {
		proc.Signal(signal)
	}

	select {
	case <-wgDone:
		level.Debug(r.logger).Log("msg", fmt.Sprintf("all desktop processes shutdown successfully with %s", signal))
		return
	case <-time.After(r.procsWgTimeout):
		level.Error(r.logger).Log("msg", "timeout waiting for desktop processes to exit with SIGTERM, now killing")
		for _, proc := range r.uidProcs {
			if !processExists(proc.Pid) {
				continue
			}
			if err := proc.Kill(); err != nil {
				level.Error(r.logger).Log(
					"msg", "error killing desktop process",
					"err", err,
					"pid", proc.Pid,
				)
			}
		}
	}
}

// waitForProcess adds 1 to DesktopUserProcessRunner.procsWg and runs a goroutine to wait on the process to exit.
// The go routine will decrement DesktopUserProcessRunner.procsWg when it exits
func (r *DesktopUsersProcessesRunner) waitOnProcessAsync(uid string, proc *os.Process) {
	r.procsWg.Add(1)
	go func(username string, proc *os.Process) {
		defer r.procsWg.Done()
		// if the desktop process dies, the parent must clean up otherwise we get a zombie process
		// waiting here gives the parent a chance to clean up
		_, err := proc.Wait()
		if err != nil {
			level.Error(r.logger).Log(
				"msg", "desktop process died",
				"uid", uid,
				"pid", proc.Pid,
				"err", err,
			)
		}
	}(uid, proc)
}

// determineExecutablePath returns DesktopUsersProcessesRunner.executablePath if it is set,
// otherwise it returns the path to the current binary.
func (r *DesktopUsersProcessesRunner) determineExecutablePath() (string, error) {
	if r.executablePath != "" {
		return r.executablePath, nil
	}

	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("error getting executable path: %w", err)
	}

	return executable, nil
}

func (r *DesktopUsersProcessesRunner) userHasDesktopProcess(uid string) bool {
	// have no record of process
	proc, ok := r.uidProcs[uid]
	if !ok {
		return false
	}

	// have a record of process, but it died for some reason, log it
	if !processExists(proc.Pid) {
		level.Info(r.logger).Log(
			"msg", "found existing desktop process dead for console user",
			"dead_pid", r.uidProcs[uid].Pid,
			"uid", uid,
		)

		return false
	}

	// have running process
	return true
}

func processExists(pid int) bool {
	isExists, err := process.PidExists(int32(pid))
	if err != nil {
		return false
	}
	return isExists
}
