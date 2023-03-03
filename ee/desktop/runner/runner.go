// runner handles multiuser process management for launcher desktop
package runner

import (
	"bufio"
	"bytes"
	"context"
	"errors"
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
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/ee/consoleuser"
	"github.com/kolide/launcher/ee/desktop/client"
	"github.com/kolide/launcher/ee/desktop/menu"
	"github.com/kolide/launcher/ee/desktop/notify"
	"github.com/kolide/launcher/ee/ui/assets"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/shirou/gopsutil/process"
	"golang.org/x/exp/maps"
)

type desktopUsersProcessesRunnerOption func(*DesktopUsersProcessesRunner)

// WithHostname sets the hostname for the runner
func WithHostname(hostname string) desktopUsersProcessesRunnerOption {
	return func(r *DesktopUsersProcessesRunner) {
		r.hostname = hostname
	}
}

func WithLogger(logger log.Logger) desktopUsersProcessesRunnerOption {
	return func(r *DesktopUsersProcessesRunner) {
		r.logger = log.With(logger,
			"component", "desktop_users_processes_runner",
		)
	}
}

// WithUpdateInterval sets the interval on which the runner will create desktops for
// user who don't have them and spin up new ones if any have died.
func WithUpdateInterval(interval time.Duration) desktopUsersProcessesRunnerOption {
	return func(r *DesktopUsersProcessesRunner) {
		r.updateInterval = interval
	}
}

// WithExecutablePath sets the path to the executable that will be run for each desktop.
func WithExecutablePath(path string) desktopUsersProcessesRunnerOption {
	return func(r *DesktopUsersProcessesRunner) {
		r.executablePath = path
	}
}

// WithInterruptTimeout sets the timeout for the runner to wait for processes to exit when interrupted.
func WithInterruptTimeout(timeout time.Duration) desktopUsersProcessesRunnerOption {
	return func(r *DesktopUsersProcessesRunner) {
		r.interruptTimeout = timeout
	}
}

// WithAuthToken sets the auth token for the runner
func WithAuthToken(token string) desktopUsersProcessesRunnerOption {
	return func(r *DesktopUsersProcessesRunner) {
		r.authToken = token
	}
}

// WithUsersFilesRoot sets the launcher root dir with will be the parent dir
// for kolide desktop files on a per user basis
func WithUsersFilesRoot(usersFilesRoot string) desktopUsersProcessesRunnerOption {
	return func(r *DesktopUsersProcessesRunner) {
		r.usersFilesRoot = usersFilesRoot
	}
}

// WithProcessSpawningEnabled sets desktop GUI enablement
func WithProcessSpawningEnabled(enabled bool) desktopUsersProcessesRunnerOption {
	return func(r *DesktopUsersProcessesRunner) {
		r.processSpawningEnabled = enabled
	}
}

// WithGetter sets the key/value getter for agent flags
func WithGetter(getter types.Getter) desktopUsersProcessesRunnerOption {
	return func(r *DesktopUsersProcessesRunner) {
		r.flagsGetter = getter
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
	// usersFilesRoot is the launcher root dir with will be the parent dir
	// for kolide desktop files on a per user basis
	usersFilesRoot string
	// processSpawningEnabled controls whether or not desktop user processes are automatically spawned
	// This effectively represents whether or not the launcher desktop GUI is enabled or not
	processSpawningEnabled bool
	// flagsGetter gets agent flags
	flagsGetter types.Getter
}

// processRecord is used to track spawned desktop processes.
// The path is used to ensure another process has not taken the same pid.
// The existence of a process record does not mean the process is running.
// If, for example, a user logs out, the process record will remain until the
// user logs in again and it is replaced.
type processRecord struct {
	process    *os.Process
	path       string
	socketPath string
}

// New creates and returns a new DesktopUsersProcessesRunner runner and initializes all required fields
func New(opts ...desktopUsersProcessesRunnerOption) *DesktopUsersProcessesRunner {
	runner := &DesktopUsersProcessesRunner{
		logger:                 log.NewNopLogger(),
		interrupt:              make(chan struct{}),
		uidProcs:               make(map[string]processRecord),
		updateInterval:         time.Second * 5,
		procsWg:                &sync.WaitGroup{},
		interruptTimeout:       time.Second * 10,
		usersFilesRoot:         agent.TempPath("kolide-desktop"),
		processSpawningEnabled: false,
	}

	for _, opt := range opts {
		opt(runner)
	}

	runner.writeIconFile()
	runner.writeDefaultMenuTemplateFile()

	// We re-generate the menu file each time launcher starts up, so it's always up-to-date
	if err := runner.generateMenuFile(); err != nil {
		if agent.Flags.DebugServerData() {
			level.Error(runner.logger).Log(
				"msg", "failed to generate menu file",
				"error", err,
			)
		}
	}

	return runner
}

// Execute immediately checks if the current console user has a desktop process running. If not, it will start a new one.
// Then repeats based on the executionInterval.
func (r *DesktopUsersProcessesRunner) Execute() error {
	ticker := time.NewTicker(r.updateInterval)
	defer ticker.Stop()

	for {
		// Check immediately on each iteration, avoiding the initial ticker delay
		if err := r.runConsoleUserDesktop(); err != nil {
			level.Info(r.logger).Log("msg", "running console user desktop", "err", err)
		}

		select {
		case <-ticker.C:
			continue
		case <-r.interrupt:
			level.Debug(r.logger).Log("msg", "interrupt received, exiting desktop execute loop")
			return nil
		}
	}
}

// Interrupt stops creating launcher desktop processes and kills any existing ones.
// It also signals the execute loop to exit, so new desktop processes cease to spawn.
func (r *DesktopUsersProcessesRunner) Interrupt(interruptError error) {
	level.Debug(r.logger).Log(
		"msg", "sending interrupt to desktop users processes runner",
		"err", interruptError,
	)

	// Tell the execute loop to stop checking, and exit
	r.interrupt <- struct{}{}

	// Kill any desktop processes that may exist
	r.killDesktopProcesses()
}

// killDesktopProcesses kills any existing desktop processes
func (r *DesktopUsersProcessesRunner) killDesktopProcesses() {
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
		maps.Clear(r.uidProcs)
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

func (r *DesktopUsersProcessesRunner) SendNotification(n notify.Notification) error {
	if len(r.uidProcs) == 0 {
		return errors.New("cannot send notification, no child desktop processes")
	}

	errs := make([]error, 0)
	for uid, proc := range r.uidProcs {
		client := client.New(r.authToken, proc.socketPath)
		if err := client.Notify(n); err != nil {
			level.Error(r.logger).Log(
				"msg", "error sending notify command to desktop process",
				"uid", uid,
				"pid", proc.process.Pid,
				"path", proc.path,
				"err", err,
			)
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors sending notifications: %+v", errs)
	}

	return nil
}

// Update handles control server updates for the desktop-menu subsystem
func (r *DesktopUsersProcessesRunner) Update(data io.Reader) error {
	if data == nil {
		return errors.New("data is nil")
	}

	var dataCopy bytes.Buffer
	dataTee := io.TeeReader(data, &dataCopy)

	// Replace the menu template file
	dataBytes, err := io.ReadAll(dataTee)
	if err != nil {
		return fmt.Errorf("error reading control data: %w", err)
	}
	if err := r.writeSharedFile(r.menuTemplatePath(), dataBytes); err != nil {
		level.Error(r.logger).Log("msg", "menu template file did not exist, could not create it", "err", err)
	}

	// Regardless, we will write the menu data out to a file that can be grabbed by
	// any desktop user processes, either when they refresh, or when they are spawned.
	if err := r.generateMenuFile(); err != nil {
		if agent.Flags.DebugServerData() {
			level.Error(r.logger).Log(
				"msg", "failed to generate menu file",
				"error", err,
				"data", dataCopy.String(),
			)
		}
		return fmt.Errorf("failed to generate menu file: %w", err)
	}

	// Tell any running desktop user processes that they should refresh the latest menu data
	for uid, proc := range r.uidProcs {
		client := client.New(r.authToken, proc.socketPath)
		if err := client.Refresh(); err != nil {
			level.Error(r.logger).Log(
				"msg", "error sending refresh command to desktop process",
				"uid", uid,
				"pid", proc.process.Pid,
				"path", proc.path,
				"err", err,
			)
		}
	}

	return nil
}

func (r *DesktopUsersProcessesRunner) Ping() {
	// agent_flags bucket has been updated, query the flags to react to changes
	enabledRaw, err := r.flagsGetter.Get([]byte("desktop_enabled"))
	if err != nil {
		level.Debug(r.logger).Log("msg", "failed to query desktop flags", "err", err)
		return
	}

	// The presence of anything for this flag means desktop is enabled
	enabled := enabledRaw != nil

	r.processSpawningEnabled = enabled
	level.Debug(r.logger).Log("msg", "runner processSpawningEnabled:%s", strconv.FormatBool(enabled))
}

// writeSharedFile writes data to a shared file for user processes to access
func (r *DesktopUsersProcessesRunner) writeSharedFile(path string, data []byte) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}

	if err := os.Chmod(path, 0644); err != nil {
		return fmt.Errorf("os.Chmod: %w", err)
	}

	defer file.Close()
	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}

// generateMenuFile generates and writes menu data to a shared file
func (r *DesktopUsersProcessesRunner) generateMenuFile() error {
	// First generate fresh template data to use for parsing
	v := version.Version()
	td := &menu.TemplateData{
		LauncherVersion:  v.Version,
		LauncherRevision: v.Revision,
		GoVersion:        v.GoVersion,
		ServerHostname:   r.hostname,
	}

	menuTemplateFileBytes, err := os.ReadFile(r.menuTemplatePath())
	if err != nil {
		return fmt.Errorf("failed to read menu template file: %w", err)
	}

	// Convert the raw JSON to a string and feed it to the parser for template expansion
	parser := menu.NewTemplateParser(td)
	parsedMenuDataStr, err := parser.Parse(string(menuTemplateFileBytes))
	if err != nil {
		return fmt.Errorf("failed to parse menu data: %w", err)
	}

	// Convert the parsed string back to bytes, which can now be decoded per usual
	parsedMenuDataBytes := []byte(parsedMenuDataStr)

	// Write the menu data out to a file that can be grabbed by
	// any desktop user processes, either when they refresh, or when they are spawned.
	if err := r.writeSharedFile(r.menuPath(), parsedMenuDataBytes); err != nil {
		return err
	}

	return nil
}

// writeDefaultMenuTemplateFile will create the menu template file, if it does not already exist
func (r *DesktopUsersProcessesRunner) writeDefaultMenuTemplateFile() {
	menuTemplatePath := r.menuTemplatePath()
	_, err := os.Stat(menuTemplatePath)

	if os.IsNotExist(err) {
		if err := r.writeSharedFile(menuTemplatePath, menu.InitialMenu); err != nil {
			level.Error(r.logger).Log("msg", "menu template file did not exist, could not create it", "err", err)
		}
	} else if err != nil {
		level.Error(r.logger).Log("msg", "could not check if menu template file exists", "err", err)
	}
}

func (r *DesktopUsersProcessesRunner) runConsoleUserDesktop() error {
	if !r.processSpawningEnabled {
		// Desktop is disabled, kill any existing desktop user processes
		r.killDesktopProcesses()
		return nil
	}

	executablePath, err := r.determineExecutablePath()
	if err != nil {
		return fmt.Errorf("determining executable path: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	consoleUsers, err := consoleuser.CurrentUids(ctx)
	if err != nil {
		return fmt.Errorf("getting console users: %w", err)
	}

	for _, uid := range consoleUsers {
		if r.userHasDesktopProcess(uid) {
			continue
		}

		socketPath, err := r.socketPath(uid)
		if err != nil {
			return fmt.Errorf("getting socket path: %w", err)
		}

		menuPath := r.menuPath()

		cmd, err := r.desktopCommand(executablePath, uid, socketPath, menuPath)
		if err != nil {
			return fmt.Errorf("creating desktop command: %w", err)
		}

		if err := r.runAsUser(uid, cmd); err != nil {
			return fmt.Errorf("running desktop command as user: %w", err)
		}

		r.waitOnProcessAsync(uid, cmd.Process)

		client := client.New(r.authToken, socketPath)
		if err := backoff.WaitFor(client.Ping, 10*time.Second, 1*time.Second); err != nil {
			if err := cmd.Process.Kill(); err != nil {
				level.Error(r.logger).Log(
					"msg", "killing desktop process after startup ping failed",
					"uid", uid,
					"pid", cmd.Process.Pid,
					"path", cmd.Path,
					"err", err,
				)
			}
			return fmt.Errorf("pinging desktop server after startup: %w", err)
		}

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
			level.Info(r.logger).Log(
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

// socketPath returns standard pipe path for windows.
// On posix systems, it creates a folder and changes owner to the user
// then provides a path to the socket in that folder
func (r *DesktopUsersProcessesRunner) socketPath(uid string) (string, error) {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`\\.\pipe\kolide_desktop_%s`, ulid.New()), nil
	}

	userFolderPath := filepath.Join(r.usersFilesRoot, fmt.Sprintf("desktop_%s", uid))
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

// menuPath returns the path to the menu file
func (r *DesktopUsersProcessesRunner) menuPath() string {
	return filepath.Join(r.usersFilesRoot, "menu.json")
}

// menuTemplatePath returns the path to the menu template file
func (r *DesktopUsersProcessesRunner) menuTemplatePath() string {
	return filepath.Join(r.usersFilesRoot, "menu_template.json")
}

// desktopCommand invokes the launcher desktop executable with the appropriate env vars
func (r *DesktopUsersProcessesRunner) desktopCommand(executablePath, uid, socketPath, menuPath string) (*exec.Cmd, error) {
	cmd := exec.Command(executablePath, "desktop")

	cmd.Env = []string{
		// When we set cmd.Env (as we're doing here/below), cmd will no longer include the default cmd.Environ()
		// when running the command. We need PATH (e.g. to be able to look up powershell and xdg-open) in the
		// desktop process, so we set it explicitly here.
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
		// without passing the temp var, the desktop icon will not appear on windows and emit the error:
		// unable to write icon data to temp file: open C:\\windows\\systray_temp_icon_...: Access is denied
		fmt.Sprintf("TEMP=%s", os.Getenv("TEMP")),
		fmt.Sprintf("HOSTNAME=%s", r.hostname),
		fmt.Sprintf("AUTHTOKEN=%s", r.authToken),
		fmt.Sprintf("SOCKET_PATH=%s", socketPath),
		fmt.Sprintf("ICON_PATH=%s", r.iconFileLocation()),
		fmt.Sprintf("MENU_PATH=%s", menuPath),
		fmt.Sprintf("PPID=%d", os.Getpid()),
	}

	stdErr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("getting stderr pipe: %w", err)
	}

	stdOut, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("getting stdout pipe: %w", err)
	}

	go func() {
		combined := io.MultiReader(stdErr, stdOut)
		scanner := bufio.NewScanner(combined)

		for scanner.Scan() {
			level.Info(r.logger).Log(
				"uid", uid,
				"subprocess", "desktop",
				"msg", scanner.Text(),
			)
		}
	}()

	return cmd, nil
}

func (r *DesktopUsersProcessesRunner) writeIconFile() {
	expectedLocation := r.iconFileLocation()

	_, err := os.Stat(expectedLocation)

	if os.IsNotExist(err) {
		if err := os.WriteFile(expectedLocation, assets.MenubarDefaultLightmodeIco, 0644); err != nil {
			level.Error(r.logger).Log("msg", "icon file did not exist, could not create it", "err", err)
		}
	} else if err != nil {
		level.Error(r.logger).Log("msg", "could not check if icon file exists", "err", err)
	}
}

func iconFilename() string {
	if runtime.GOOS == "windows" {
		return "kolide.ico"
	}
	return "kolide.png"
}

func (r *DesktopUsersProcessesRunner) iconFileLocation() string {
	return filepath.Join(r.usersFilesRoot, iconFilename())
}
