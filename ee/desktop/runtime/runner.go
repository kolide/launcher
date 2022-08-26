package runtime

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
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
		if err := r.runConsoleUserDesktop(); err != nil {
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
