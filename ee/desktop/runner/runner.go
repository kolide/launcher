// runtime handles multiuser process management for launcher desktop
package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/desktop/client"
	"github.com/shirou/gopsutil/process"
)

type DesktopUsersProcessesRunnerOption func(*DesktopUsersProcessesRunner)

// WithHostname sets the hostname for the runner
func WithHostname(hostname string) DesktopUsersProcessesRunnerOption {
	return func(r *DesktopUsersProcessesRunner) {
		r.hostname = hostname
	}
}

func WithLogger(logger log.Logger) DesktopUsersProcessesRunnerOption {
	return func(r *DesktopUsersProcessesRunner) {
		r.logger = logger
	}
}

// WithUpdateInterval sets the interval on which the runner will create desktops for
// user who don't have them and spin up new ones if any have died.
func WithUpdateInterval(interval time.Duration) DesktopUsersProcessesRunnerOption {
	return func(r *DesktopUsersProcessesRunner) {
		r.updateInterval = interval
	}
}

// WithExecutablePath sets the path to the executable that will be run for each desktop.
func WithExecutablePath(path string) DesktopUsersProcessesRunnerOption {
	return func(r *DesktopUsersProcessesRunner) {
		r.executablePath = path
	}
}

// WithInterruptTimeout sets the timeout for the runner to wait for processes to exit when interrupted.
func WithInterruptTimeout(timeout time.Duration) DesktopUsersProcessesRunnerOption {
	return func(r *DesktopUsersProcessesRunner) {
		r.interruptTimeout = timeout
	}
}

// WithAuth sets the auth token for the runner
func WithAuthToken(token string) DesktopUsersProcessesRunnerOption {
	return func(r *DesktopUsersProcessesRunner) {
		r.authToken = token
	}
}

// WithLauncherRootDir sets the launcher root dir with will be the parent dir
// for kolide desktop files on a per user basis
func WithLauncherRootDir(token string) DesktopUsersProcessesRunnerOption {
	return func(r *DesktopUsersProcessesRunner) {
		r.launcherRootDir = token
	}
}

// DesktopUsersProcessesRunner creates a launcher desktop process each time it detects
// a new console (GUI) user. If the current console user's desktop process dies, it
// will create a new one.
// Initialize with New().
type DesktopUsersProcessesRunner struct {
	logger         log.Logger
	updateInterval time.Duration
	interrupt      chan struct{}
	// uidProcs is a map of uid to desktop process
	uidProcs map[string]processRecord
	// procsWg is a WaitGroup to wait for all desktop processes to finish during an interrupt
	procsWg *sync.WaitGroup
	// interruptTimeout how long to wait for desktop proccesses to finish on interrupt
	interruptTimeout time.Duration
	// executablePath is the path to the launcher executable. Currently this is only set during testing
	// due to needing to build the binary to test as a result of some test harness weirdness.
	// See runner_test.go for more details.
	executablePath string
	// hostname is the host that launcher is connecting to. It gets passed to the desktop process
	// and is used to determine which icon to display
	hostname string
	// authToken is the auth token to use when connecting to the launcher desktop server
	authToken string
	// launcherRootDir is the launcher root dir with will be the parent dir
	// for kolide desktop files on a per user basis
	launcherRootDir string
}

// processRecord is a struct to hold an *os.process and its path.
// The path is used to ensure another process has not taken the same pid.
type processRecord struct {
	process    *os.Process
	path       string
	socketPath string
}

// New creates and returns a new DesktopUsersProcessesRunner runner and initializes all required fields
func New(opts ...DesktopUsersProcessesRunnerOption) *DesktopUsersProcessesRunner {
	runner := &DesktopUsersProcessesRunner{
		logger:           log.NewNopLogger(),
		interrupt:        make(chan struct{}),
		uidProcs:         make(map[string]processRecord),
		updateInterval:   time.Second * 5,
		procsWg:          &sync.WaitGroup{},
		interruptTimeout: time.Second * 10,
		launcherRootDir:  filepath.Join(os.TempDir(), "launcher"),
	}

	for _, opt := range opts {
		opt(runner)
	}

	return runner
}

// Execute immediately checks if the current console user has a desktop process running. If not, it will start a new one.
// Then repeats based on the executionInterval.
func (r *DesktopUsersProcessesRunner) Execute() error {
	f := func() {
		if err := r.runConsoleUserDesktop(); err != nil {
			level.Error(r.logger).Log("msg", "running console user desktop", "err", err)
		}
	}

	f()

	ticker := time.NewTicker(r.updateInterval)

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

	for uid, proc := range r.uidProcs {
		client := client.New(r.authToken, proc.socketPath)
		if err := client.Shutdown(); err != nil {
			level.Error(r.logger).Log(
				"msg", "error sending shutdown command to desktop process",
				"uid", uid,
				"pid", proc.process.Pid,
				"path", proc.path,
				"err", err,
			)
		}
	}

	select {
	case <-wgDone:
		level.Debug(r.logger).Log("msg", "all desktop processes shutdown successfully")
		return
	case <-time.After(r.interruptTimeout):
		level.Error(r.logger).Log("msg", "timeout waiting for desktop processes to exit, now killing")
		for uid, processRecord := range r.uidProcs {
			if !processExists(processRecord) {
				continue
			}
			if err := processRecord.process.Kill(); err != nil {
				level.Error(r.logger).Log(
					"msg", "error killing desktop process",
					"uid", uid,
					"pid", processRecord.process.Pid,
					"path", processRecord.path,
					"err", err,
				)
			}
		}
	}
}

func (r *DesktopUsersProcessesRunner) runConsoleUserDesktop() error {
	executablePath, err := r.determineExecutablePath()
	if err != nil {
		return fmt.Errorf("determining executable path: %w", err)
	}

	consolerUsers, err := r.consoleUsers()
	if err != nil {
		return fmt.Errorf("getting console users: %w", err)
	}

	for _, uid := range consolerUsers {
		if r.userHasDesktopProcess(uid) {
			continue
		}

		socketPath, err := r.socketPath(uid)
		if err != nil {
			return fmt.Errorf("getting socket path: %w", err)
		}

		cmd, err := cmdAsUser(uid, executablePath, "desktop")
		if err != nil {
			return fmt.Errorf("creating desktop command: %w", err)
		}

		if runtime.GOOS == "windows" {
			defer cmd.SysProcAttr.Token.Close()
		}

		cmd.Env = r.processEnvVars(socketPath)

		if err := r.startDesktopCommand(uid, cmd); err != nil {
			return fmt.Errorf("starting desktop command: %w", err)
		}

		r.waitOnProcessAsync(uid, cmd.Process)

		level.Debug(r.logger).Log(
			"msg", "desktop started",
			"uid", uid,
			"pid", cmd.Process.Pid,
		)

		if err := r.addProcessTrackingRecordForUser(uid, socketPath, cmd.Process); err != nil {
			return fmt.Errorf("adding process to internal tracking state: %w", err)
		}
	}

	return nil
}

// addProcessTrackingRecordForUser adds process information to the internal tracking state
func (r *DesktopUsersProcessesRunner) addProcessTrackingRecordForUser(uid string, socketPath string, osProcess *os.Process) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	psutilProc, err := process.NewProcessWithContext(ctx, int32(osProcess.Pid))
	if err != nil {
		return fmt.Errorf("creating process record: %w", err)
	}

	path, err := psutilProc.ExeWithContext(ctx)
	if err != nil {
		return fmt.Errorf("getting process path: %w", err)
	}

	r.uidProcs[uid] = processRecord{
		process:    osProcess,
		path:       path,
		socketPath: socketPath,
	}

	return nil
}

// waitForProcess adds 1 to DesktopUserProcessRunner.procsWg and runs a goroutine to wait on the process to exit.
// The go routine will decrement DesktopUserProcessRunner.procsWg when it exits. This is necessary because if
// the process dies and we do not wait for it, it will live as a zombie and not get cleaned up by the parent.
// The wait group is needed to prevent races.
func (r *DesktopUsersProcessesRunner) waitOnProcessAsync(uid string, proc *os.Process) {
	r.procsWg.Add(1)
	go func(username string, proc *os.Process) {
		defer r.procsWg.Done()
		// waiting here gives the parent a chance to clean up
		state, err := proc.Wait()
		if err != nil {
			level.Error(r.logger).Log(
				"msg", "desktop process died",
				"uid", uid,
				"pid", proc.Pid,
				"err", err,
				"state", state,
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
	if !processExists(proc) {
		level.Info(r.logger).Log(
			"msg", "found existing desktop process dead for console user",
			"pid", r.uidProcs[uid].process.Pid,
			"process_path", r.uidProcs[uid].path,
			"uid", uid,
		)

		return false
	}

	// have running process
	return true
}

func processExists(processRecord processRecord) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	// the call to process.NewProcessWithContext ensures process exists
	proc, err := process.NewProcessWithContext(ctx, int32(processRecord.process.Pid))
	if err != nil {
		return false
	}

	path, err := proc.ExeWithContext(ctx)
	if err != nil || path != processRecord.path {
		return false
	}

	return true
}

func (r *DesktopUsersProcessesRunner) processEnvVars(socketPath string) []string {
	const varFmt = "%s=%s"
	return append(
		os.Environ(),
		fmt.Sprintf(varFmt, "HOSTNAME", r.hostname),
		fmt.Sprintf(varFmt, "AUTHTOKEN", r.authToken),
		fmt.Sprintf(varFmt, "SOCKET_PATH", socketPath),
	)
}

// socketPath returns standard pipe path for windows
// on posix systems, it creates a folder and changes owner to the user
// then provides a path to the socket in that folder
func (r *DesktopUsersProcessesRunner) socketPath(uid string) (string, error) {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`\\.\pipe\kolide_desktop_%s`, ulid.New()), nil
	}

	userFolderPath := filepath.Join(r.launcherRootDir, fmt.Sprintf("desktop_%s", uid))
	if err := os.MkdirAll(userFolderPath, 0700); err != nil {
		return "", fmt.Errorf("creating user folder: %w", err)
	}

	uidInt, err := strconv.Atoi(uid)
	if err != nil {
		return "", fmt.Errorf("converting uid to int: %w", err)
	}

	if err := os.Chown(userFolderPath, uidInt, -1); err != nil {
		return "", fmt.Errorf("chowning user folder: %w", err)
	}

	path := filepath.Join(userFolderPath, "kolide_desktop.sock")
	const maxSocketLength = 103
	if len(path) > maxSocketLength {
		return "", fmt.Errorf("socket path %s (length %d) is too long, max is %d", path, len(path), maxSocketLength)
	}

	return path, nil
}

func (r *DesktopUsersProcessesRunner) startDesktopCommand(uid string, cmd *exec.Cmd) error {
	stdErr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("getting stderr pipe: %w", err)
	}

	stdOut, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("getting stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting desktop process: %w", err)
	}

	go func() {
		combined := io.MultiReader(stdErr, stdOut)
		scanner := bufio.NewScanner(combined)

		for scanner.Scan() {
			level.Info(r.logger).Log(
				"uid", uid,
				"pid", cmd.Process.Pid,
				"msg", scanner.Text(),
			)
		}
	}()

	return nil
}
