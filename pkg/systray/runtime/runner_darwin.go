//go:build darwin
// +build darwin

package runtime

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"

	"github.com/go-kit/kit/log/level"
)

func (r *SystrayUsersProcessesRunner) runConsoleUserSystray() error {
	consoleOwnerUid, err := consoleOwnerUid()
	if err != nil {
		return fmt.Errorf("getting console owner uid: %w", err)
	}

	// there seems to be a brief moment during start up where root or system (non-human)
	// users own the console, if we spin up the process for them it will add an
	// unnecessary process. On macOS human users start at 501
	if consoleOwnerUid < 500 {
		level.Debug(r.logger).Log(
			"msg", "skipping systray for root or system user",
			"uid", consoleOwnerUid,
		)

		return nil
	}

	uid := fmt.Sprint(consoleOwnerUid)

	// already have a systray for the console owner
	if proc, ok := r.uidProcs[uid]; ok {
		// if the process is still running, return
		if processExists(proc.Pid) {
			return nil
		}

		// proc is dead
		level.Info(r.logger).Log(
			"msg", "existing systray process dead for console user, starting new systray process",
			"dead_pid", r.uidProcs[uid].Pid,
			"uid", consoleOwnerUid,
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
	r.uidProcs[uid] = proc

	level.Debug(r.logger).Log(
		"msg", "systray started",
		"uid", consoleOwnerUid,
		"pid", proc.Pid,
	)

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

	uidInt, err := strconv.ParseUint(uid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("converting uid to int: %w", err)
	}

	gid, err := strconv.ParseUint(runningUser.Gid, 10, 32)
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

func processExists(pid int) bool {
	// this code was adapted from https://github.com/shirou/gopsutil/blob/ed37dc27a286a25cbe76adf405176c69191a1f37/process/process_posix.go#L102
	// thank you shirou!
	if pid <= 0 {
		return false
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// from kill 1 man: If sig is 0, then no signal is sent, but error checking is still performed.
	// bash equivalent of: kill -n 0 <pid>
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}

	if err.Error() == "os: process already finished" {
		return false
	}

	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}

	switch errno {
	// No process or process group can be found corresponding to that specified by pid.
	case syscall.ESRCH:
		return false
	// The sending process is not the super-user and its effective user id does not match the effective user-id of the receiving process.
	// When signaling a process group, this error is returned if any members of the group could not be signaled.
	case syscall.EPERM:
		return true
	}

	return false
}
