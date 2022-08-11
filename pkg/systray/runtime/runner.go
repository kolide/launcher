package runtime

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

// SystrayUserProcessRunner creates a launcher systray process each time it detects
// a new console (GUI) user. If the current console user's systray process dies, it
// will create a new one.
// Initalize with NewSystrayUserProcessRunner().
type SystrayUserProcessRunner struct {
	logger            log.Logger
	executionInterval time.Duration
	interrupt         chan struct{}
	// uidProcs is a map of uid to systray process
	uidProcs map[string]*os.Process
}

// NewSystrayUserProcessRunner creates and returns a new SystrayUserProcess runner and initializes all required fields
func NewSystrayUserProcessRunner(logger log.Logger) *SystrayUserProcessRunner {
	return &SystrayUserProcessRunner{
		logger:            logger,
		interrupt:         make(chan struct{}),
		uidProcs:          make(map[string]*os.Process),
		executionInterval: time.Second * 5,
	}
}

// Execute checks if the current console user has a systray process running. If not, it will start a new one.
func (r *SystrayUserProcessRunner) Execute() error {
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

// Interrupt stops creating launcher systray processes and kils the existing ones.
func (r *SystrayUserProcessRunner) Interrupt(err error) {
	r.interrupt <- struct{}{}
	for _, proc := range r.uidProcs {
		if isExists, _ := processExists(proc.Pid); isExists {
			if err := proc.Kill(); err != nil {
				level.Error(r.logger).Log(
					"msg", "error killing systray",
					"err", err,
				)
			}
		}
	}
}

func (r *SystrayUserProcessRunner) runConsoleUserSystray() error {
	consoleOwnerUid, err := consoleOwnerUid()
	if err != nil {
		return fmt.Errorf("getting console owner uid: %w", err)
	}

	// there seems to be a brief moment during start up where root has the console
	// if we spin up the process for root after the user gets the console it will
	// add another systray icon, so don't spin it up for root
	if consoleOwnerUid == 0 {
		level.Info(r.logger).Log(
			"msg", "skipping systray for root user",
			"uid", consoleOwnerUid,
		)

		return nil
	}

	uid := fmt.Sprintf("%d", consoleOwnerUid)

	// already have a systray for the console owner
	if _, ok := r.uidProcs[uid]; ok {
		// if the process is still running, return
		if isExists, _ := processExists(r.uidProcs[uid].Pid); isExists {
			return nil
		}

		// proc is dead
		level.Info(r.logger).Log(
			"msg", "existing systray process dead for console user, starting new systray process",
			"dead_pid", r.uidProcs[uid].Pid,
			"uid", uid,
		)
	}

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}

	proc, err := runAsUser(uid, executable, "systray")
	if err != nil {
		return fmt.Errorf("running systray: %w", err)
	}

	level.Info(r.logger).Log(
		"msg", "systray started",
		"uid", uid,
		"pid", proc.Pid,
	)

	r.uidProcs[uid] = proc
	return nil
}

func consoleOwnerUid() (uint32, error) {
	const consolePath = "/dev/console"
	consoleInfo, err := os.Stat(consolePath)
	if err != nil {
		return uint32(0), fmt.Errorf("stat %s: %w", consolePath, err)
	}

	return consoleInfo.Sys().(*syscall.Stat_t).Uid, nil
}

func runAsUser(uid string, path string, args ...string) (*os.Process, error) {
	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("getting current user: %w", err)
	}

	runningUser, err := user.LookupId(uid)
	if err != nil {
		return nil, fmt.Errorf("looking up user with uid %s: %w", uid, err)
	}

	cmd := exec.Command(path, args...)

	// current user not root
	if currentUser.Uid != "0" {

		// if the user is running for itself, just run without setting credentials
		if currentUser.Uid == uid {
			err := cmd.Start()
			if err != nil {
				return nil, fmt.Errorf("running command: %w", err)
			}
			return cmd.Process, nil
		}

		// if the user is running for another user, we have an error because we can't set credentials
		return nil, fmt.Errorf("current user %s is not root and cant start process for other user %s", currentUser.Uid, uid)
	}

	// the remaining code in this function is not covered by unit test since it requires root privileges
	// We may be able to run passwordless sudo in GitHub actions, could possibly exec the tests as sudo.
	// But we may not have a console user?

	uidInt, err := strconv.Atoi(runningUser.Uid)
	if err != nil {
		return nil, fmt.Errorf("converting uid to int: %w", err)
	}

	gid, err := strconv.Atoi(runningUser.Gid)
	if err != nil {
		return nil, fmt.Errorf("converting gid to int: %w", err)
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uint32(uidInt),
			Gid: uint32(gid),
		},
	}
	err = cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("starting command: %w", err)
	}

	return cmd.Process, nil
}

func processExists(pid int) (bool, error) {
	// this code was adapted from https://github.com/shirou/gopsutil/blob/ed37dc27a286a25cbe76adf405176c69191a1f37/process/process_posix.go#L102
	// thank you shirou!
	if pid <= 0 {
		return false, fmt.Errorf("invalid pid %v", pid)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, err
	}

	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true, nil
	}

	if err.Error() == "os: process already finished" {
		return false, nil
	}

	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false, err
	}

	switch errno {
	case syscall.ESRCH:
		return false, nil
	case syscall.EPERM:
		return true, nil
	}

	return false, err
}
