package runtime

import (
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

// SystrayUsersProcessesRunner creates a launcher systray process each time it detects
// a new console (GUI) user. If the current console user's systray process dies, it
// will create a new one.
// Initialize with New().
type SystrayUsersProcessesRunner struct {
	logger            log.Logger
	executionInterval time.Duration
	interrupt         chan struct{}
	// uidProcs is a map of uid to systray process
	uidProcs map[string]*os.Process
}

// New creates and returns a new SystrayUsersProcessesRunner runner and initializes all required fields
func New(logger log.Logger, executionInterval time.Duration) *SystrayUsersProcessesRunner {
	return &SystrayUsersProcessesRunner{
		logger:            logger,
		interrupt:         make(chan struct{}),
		uidProcs:          make(map[string]*os.Process),
		executionInterval: executionInterval,
	}
}

// Execute immediately checks if the current console user has a systray process running. If not, it will start a new one.
// Then repeats based on the executionInterval.
func (r *SystrayUsersProcessesRunner) Execute() error {
	f := func() {
		if err := r.runConsoleUserSystray(); err != nil {
			level.Error(r.logger).Log(
				"msg", "error running systray",
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
			return nil
		}
	}
}

// Interrupt stops creating launcher systray processes and kills any existing ones.
func (r *SystrayUsersProcessesRunner) Interrupt(err error) {
	r.interrupt <- struct{}{}
	for _, proc := range r.uidProcs {
		if !processExists(proc.Pid) {
			continue
		}

		if err := proc.Kill(); err != nil {
			level.Error(r.logger).Log(
				"msg", "error killing systray process after interrupt",
				"err", err,
			)
		}
	}
}
