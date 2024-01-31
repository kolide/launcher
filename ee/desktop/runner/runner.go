// runner handles multiuser process management for launcher desktop
package runner

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/consoleuser"
	runnerserver "github.com/kolide/launcher/ee/desktop/runner/server"
	"github.com/kolide/launcher/ee/desktop/user/client"
	"github.com/kolide/launcher/ee/desktop/user/menu"
	"github.com/kolide/launcher/ee/desktop/user/notify"
	"github.com/kolide/launcher/ee/ui/assets"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/shirou/gopsutil/v3/process"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

const nonWindowsDesktopSocketPrefix = "desktop.sock"

type desktopUsersProcessesRunnerOption func(*DesktopUsersProcessesRunner)

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
		r.userServerAuthToken = token
	}
}

// WithUsersFilesRoot sets the launcher root dir with will be the parent dir
// for kolide desktop files on a per user basis
func WithUsersFilesRoot(usersFilesRoot string) desktopUsersProcessesRunnerOption {
	return func(r *DesktopUsersProcessesRunner) {
		r.usersFilesRoot = usersFilesRoot
	}
}

var instance *DesktopUsersProcessesRunner
var instanceSet = &sync.Once{}

func setInstance(r *DesktopUsersProcessesRunner) {
	instanceSet.Do(func() {
		instance = r
	})
}

func InstanceDesktopProcessRecords() map[string]processRecord {
	if instance == nil {
		return nil
	}

	return instance.uidProcs
}

// DesktopUsersProcessesRunner creates a launcher desktop process each time it detects
// a new console (GUI) user. If the current console user's desktop process dies, it
// will create a new one.
// Initialize with New().
type DesktopUsersProcessesRunner struct {
	slogger *slog.Logger
	// updateInterval is the interval on which desktop processes will be spawned, if necessary
	updateInterval time.Duration
	// menuRefreshInterval is the interval on which the desktop menu will be refreshed
	menuRefreshInterval time.Duration
	interrupt           chan struct{}
	interrupted         bool
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
	// userServerAuthToken is the auth token to use when connecting to the launcher user server
	userServerAuthToken string
	// usersFilesRoot is the launcher root dir with will be the parent dir
	// for kolide desktop files on a per user basis
	usersFilesRoot string
	// processSpawningEnabled controls whether or not desktop user processes are automatically spawned
	// This effectively represents whether or not the launcher desktop GUI is enabled or not
	processSpawningEnabled bool
	// knapsack is the almighty sack of knaps
	knapsack types.Knapsack
	// runnerServer is a local server that desktop processes call to monitor parent
	runnerServer *runnerserver.RunnerServer
	// osVersion is the version of the OS cached in new
	osVersion string
}

// processRecord is used to track spawned desktop processes.
// The path is used to ensure another process has not taken the same pid.
// The existence of a process record does not mean the process is running.
// If, for example, a user logs out, the process record will remain until the
// user logs in again and it is replaced.
type processRecord struct {
	Process                    *os.Process
	StartTime, LastHealthCheck time.Time
	path                       string
	socketPath                 string
}

func (pr processRecord) String() string {
	return fmt.Sprintf("%s [socket: %s, started: %s, last_health_check: %s])",
		pr.path,
		pr.socketPath,
		pr.StartTime.String(),
		pr.LastHealthCheck.String(),
	)
}

// New creates and returns a new DesktopUsersProcessesRunner runner and initializes all required fields
func New(k types.Knapsack, messenger runnerserver.Messenger, opts ...desktopUsersProcessesRunnerOption) (*DesktopUsersProcessesRunner, error) {
	runner := &DesktopUsersProcessesRunner{
		interrupt:              make(chan struct{}),
		uidProcs:               make(map[string]processRecord),
		updateInterval:         k.DesktopUpdateInterval(),
		menuRefreshInterval:    k.DesktopMenuRefreshInterval(),
		procsWg:                &sync.WaitGroup{},
		interruptTimeout:       time.Second * 10,
		hostname:               k.KolideServerURL(),
		usersFilesRoot:         agent.TempPath("kolide-desktop"),
		processSpawningEnabled: k.DesktopEnabled(),
		knapsack:               k,
	}

	runner.slogger = k.Slogger().With("component", "desktop_runner")

	for _, opt := range opts {
		opt(runner)
	}

	runner.writeIconFile()
	runner.writeDefaultMenuTemplateFile()
	runner.refreshMenu()

	// Observe DesktopEnabled changes to know when to enable/disable process spawning
	runner.knapsack.RegisterChangeObserver(runner, keys.DesktopEnabled)

	rs, err := runnerserver.New(runner.slogger, k, messenger)
	if err != nil {
		return nil, fmt.Errorf("creating desktop runner server: %w", err)
	}

	runner.runnerServer = rs
	go func() {
		if err := runner.runnerServer.Serve(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			runner.slogger.Log(context.TODO(), slog.LevelError,
				"running monitor server",
				"err", err,
			)
		}
	}()

	if runtime.GOOS == "darwin" {
		osversion, err := osversion()
		if err != nil {
			runner.slogger.Log(context.TODO(), slog.LevelError,
				"getting os version",
				"err", err,
			)
		}
		runner.osVersion = osversion
	}

	setInstance(runner)
	return runner, nil
}

// Execute immediately checks if the current console user has a desktop process running. If not, it will start a new one.
// Then repeats based on the executionInterval.
func (r *DesktopUsersProcessesRunner) Execute() error {
	updateTicker := time.NewTicker(r.updateInterval)
	defer updateTicker.Stop()
	menuRefreshTicker := time.NewTicker(r.menuRefreshInterval)
	defer menuRefreshTicker.Stop()
	osUpdateCheckTicker := time.NewTicker(1 * time.Minute)
	defer osUpdateCheckTicker.Stop()

	for {
		// Check immediately on each iteration, avoiding the initial ticker delay
		if err := r.runConsoleUserDesktop(); err != nil {
			r.slogger.Log(context.TODO(), slog.LevelInfo,
				"running console user desktop",
				"err", err,
			)
		}

		select {
		case <-updateTicker.C:
			continue
		case <-menuRefreshTicker.C:
			r.refreshMenu()
			continue
		case <-osUpdateCheckTicker.C:
			r.checkOsUpdate()
			continue
		case <-r.interrupt:
			r.slogger.Log(context.TODO(), slog.LevelDebug,
				"interrupt received, exiting desktop execute loop",
			)
			return nil
		}
	}
}

// Interrupt stops creating launcher desktop processes and kills any existing ones.
// It also signals the execute loop to exit, so new desktop processes cease to spawn.
func (r *DesktopUsersProcessesRunner) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if r.interrupted {
		return
	}

	r.interrupted = true

	// Tell the execute loop to stop checking, and exit
	r.interrupt <- struct{}{}

	// Kill any desktop processes that may exist
	r.killDesktopProcesses()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := r.runnerServer.Shutdown(ctx); err != nil {
		r.slogger.Log(ctx, slog.LevelError,
			"shutting down monitor server",
			"err", err,
		)
	}
}

// killDesktopProcesses kills any existing desktop processes
func (r *DesktopUsersProcessesRunner) killDesktopProcesses() {
	wgDone := make(chan struct{})
	go func() {
		defer close(wgDone)
		r.procsWg.Wait()
	}()

	shutdownRequestCount := 0
	for uid, proc := range r.uidProcs {
		// unregistering client from runner server so server will not respond to its requests
		r.runnerServer.DeRegisterClient(uid)

		client := client.New(r.userServerAuthToken, proc.socketPath)
		if err := client.Shutdown(); err != nil {
			r.slogger.Log(context.TODO(), slog.LevelError,
				"sending shutdown command to user desktop process",
				"uid", uid,
				"pid", proc.Process.Pid,
				"path", proc.path,
				"err", err,
			)
			continue
		}
		shutdownRequestCount++
	}

	select {
	case <-wgDone:
		if shutdownRequestCount > 0 {
			r.slogger.Log(context.TODO(), slog.LevelDebug,
				"successfully completed desktop process shutdown requests",
				"count", shutdownRequestCount,
			)
		}

		maps.Clear(r.uidProcs)
		return
	case <-time.After(r.interruptTimeout):
		r.slogger.Log(context.TODO(), slog.LevelError,
			"timeout waiting for desktop processes to exit, now killing",
		)

		for uid, processRecord := range r.uidProcs {
			if !r.processExists(processRecord) {
				continue
			}
			if err := processRecord.Process.Kill(); err != nil {
				r.slogger.Log(context.TODO(), slog.LevelError,
					"killing desktop process",
					"uid", uid,
					"pid", processRecord.Process.Pid,
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
	for _, proc := range r.uidProcs {
		client := client.New(r.userServerAuthToken, proc.socketPath)
		if err := client.Notify(n); err != nil {
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
		r.slogger.Log(context.TODO(), slog.LevelError,
			"menu template file did not exist, could not create it",
			"err", err,
		)
	}

	// Regardless, we will write the menu data out to a file that can be grabbed by
	// any desktop user processes, either when they refresh, or when they are spawned.
	r.refreshMenu()

	return nil
}

func (r *DesktopUsersProcessesRunner) FlagsChanged(flagKeys ...keys.FlagKey) {
	if slices.Contains(flagKeys, keys.DesktopEnabled) {
		r.processSpawningEnabled = r.knapsack.DesktopEnabled()
		r.slogger.Log(context.TODO(), slog.LevelDebug,
			"runner processSpawningEnabled set by control server",
			"processSpawningEnabled", r.processSpawningEnabled,
		)
	}
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

// refreshMenu updates the menu file and tells desktop processes to refresh their menus
func (r *DesktopUsersProcessesRunner) refreshMenu() {
	if err := r.generateMenuFile(); err != nil {
		if r.knapsack.DebugServerData() {
			r.slogger.Log(context.TODO(), slog.LevelError,
				"failed to generate menu file",
				"err", err,
			)
		}
	}

	// Tell any running desktop user processes that they should refresh the latest menu data
	for uid, proc := range r.uidProcs {
		client := client.New(r.userServerAuthToken, proc.socketPath)
		if err := client.Refresh(); err != nil {

			r.slogger.Log(context.TODO(), slog.LevelError,
				"sending refresh command to user desktop process",
				"uid", uid,
				"pid", proc.Process.Pid,
				"path", proc.path,
				"err", err,
			)
		}
	}
}

// generateMenuFile generates and writes menu data to a shared file
func (r *DesktopUsersProcessesRunner) generateMenuFile() error {
	// First generate fresh template data to use for parsing
	v := version.Version()

	info, err := os.Stat(r.menuTemplatePath())
	if err != nil {
		return fmt.Errorf("failed to stat menu template file: %w", err)
	}

	td := &menu.TemplateData{
		menu.LauncherVersion:    v.Version,
		menu.LauncherRevision:   v.Revision,
		menu.GoVersion:          v.GoVersion,
		menu.ServerHostname:     r.hostname,
		menu.LastMenuUpdateTime: info.ModTime().Unix(),
		menu.MenuVersion:        menu.CurrentMenuVersion,
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
			r.slogger.Log(context.TODO(), slog.LevelError,
				"menu template file did not exist, could not create it",
				"err", err,
			)
		}
	} else if err != nil {
		r.slogger.Log(context.TODO(), slog.LevelError,
			"could not check if menu template file exists",
			"err", err,
		)
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	consoleUsers, err := consoleuser.CurrentUids(ctx)
	if err != nil {
		return fmt.Errorf("getting console users: %w", err)
	}

	for _, uid := range consoleUsers {
		if r.userHasDesktopProcess(uid) {
			continue
		}

		// we've decided to spawn a new desktop user process for this user
		if err := r.spawnForUser(ctx, uid, executablePath); err != nil {
			return fmt.Errorf("spawning new desktop user process for %s: %w", uid, err)
		}
	}

	return nil
}

func (r *DesktopUsersProcessesRunner) spawnForUser(ctx context.Context, uid string, executablePath string) error {
	ctx, span := traces.StartSpan(ctx, "uid", uid)
	defer span.End()

	// make sure any existing user desktop processes stop being
	// recognized by the runner server
	r.runnerServer.DeRegisterClient(uid)

	socketPath, err := r.setupSocketPath(uid)
	if err != nil {
		traces.SetError(span, fmt.Errorf("getting socket path: %w", err))
		return fmt.Errorf("getting socket path: %w", err)
	}

	cmd, err := r.desktopCommand(executablePath, uid, socketPath, r.menuPath())
	if err != nil {
		traces.SetError(span, fmt.Errorf("creating desktop command: %w", err))
		return fmt.Errorf("creating desktop command: %w", err)
	}

	if err := r.runAsUser(ctx, uid, cmd); err != nil {
		traces.SetError(span, fmt.Errorf("running desktop command as user: %w", err))
		return fmt.Errorf("running desktop command as user: %w", err)
	}

	span.AddEvent("command_started")

	r.waitOnProcessAsync(uid, cmd.Process)

	client := client.New(r.userServerAuthToken, socketPath)
	if err := backoff.WaitFor(client.Ping, 10*time.Second, 1*time.Second); err != nil {
		// unregister proc from desktop server so server will not respond to its requests
		r.runnerServer.DeRegisterClient(uid)

		if err := cmd.Process.Kill(); err != nil {
			r.slogger.Log(ctx, slog.LevelError,
				"killing user desktop process after startup ping failed",
				"uid", uid,
				"pid", cmd.Process.Pid,
				"path", cmd.Path,
				"err", err,
			)
		}

		traces.SetError(span, fmt.Errorf("pinging user desktop server after startup: pid %d: %w", cmd.Process.Pid, err))

		return fmt.Errorf("pinging user desktop server after startup: pid %d: %w", cmd.Process.Pid, err)
	}

	r.slogger.Log(ctx, slog.LevelDebug,
		"desktop process started",
		"uid", uid,
		"pid", cmd.Process.Pid,
	)

	span.AddEvent("desktop_started")

	if err := r.addProcessTrackingRecordForUser(uid, socketPath, cmd.Process); err != nil {
		traces.SetError(span, fmt.Errorf("adding process to internal tracking state: %w", err))
		return fmt.Errorf("adding process to internal tracking state: %w", err)
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
		Process:    osProcess,
		StartTime:  time.Now().UTC(),
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
			r.slogger.Log(context.TODO(), slog.LevelInfo,
				"desktop process died",
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
	if !r.processExists(proc) {
		r.slogger.Log(context.TODO(), slog.LevelInfo,
			"found existing desktop process dead for console user",
			"pid", proc.Process.Pid,
			"process_path", proc.path,
			"uid", uid,
		)

		return false
	}

	proc.LastHealthCheck = time.Now().UTC()
	r.uidProcs[uid] = proc

	// have running process
	return true
}

func (r *DesktopUsersProcessesRunner) processExists(processRecord processRecord) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	// the call to process.NewProcessWithContext ensures process exists
	proc, err := process.NewProcessWithContext(ctx, int32(processRecord.Process.Pid))
	if err != nil {
		r.slogger.Log(ctx, slog.LevelInfo,
			"error checking existing desktop process",
			"pid", processRecord.Process.Pid,
			"process_path", processRecord.path,
			"err", err,
		)
		return false
	}

	path, err := proc.ExeWithContext(ctx)
	if err != nil || path != processRecord.path {
		r.slogger.Log(ctx, slog.LevelInfo,
			"error or path mismatch checking existing desktop process path",
			"pid", processRecord.Process.Pid,
			"process_record_path", processRecord.path,
			"err", err,
			"found_path", path,
		)
		return false
	}

	return true
}

// setupSocketPath returns standard pipe path for windows.
// On posix systems, it creates a directory and changes owner to the user,
// deletes any existing desktop sockets in the directory,
// then provides a path to the socket in that folder.
func (r *DesktopUsersProcessesRunner) setupSocketPath(uid string) (string, error) {
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

	if err := removeFilesWithPrefix(userFolderPath, nonWindowsDesktopSocketPrefix); err != nil {
		r.slogger.Log(context.TODO(), slog.LevelInfo,
			"removing existing desktop sockets for user",
			"uid", uid,
			"err", err,
		)
	}

	// using random 4 digit number instead of ulid to keep name short so we don't
	// exceed char limit
	path := filepath.Join(userFolderPath, fmt.Sprintf("%s_%d", nonWindowsDesktopSocketPrefix, rand.Intn(10000)))
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
	cmd := exec.Command(executablePath, "desktop") //nolint:forbidigo // We trust that the launcher executable path is correct, so we don't need to use allowedcmd

	cmd.Env = []string{
		// When we set cmd.Env (as we're doing here/below), cmd will no longer include the default cmd.Environ()
		// when running the command. We need PATH (e.g. to be able to look up powershell and xdg-open) in the
		// desktop process, so we set it explicitly here.
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
		// without passing the temp var, the desktop icon will not appear on windows and emit the error:
		// unable to write icon data to temp file: open C:\\windows\\systray_temp_icon_...: Access is denied
		fmt.Sprintf("TEMP=%s", os.Getenv("TEMP")),
		fmt.Sprintf("HOSTNAME=%s", r.hostname),
		fmt.Sprintf("USER_SERVER_AUTH_TOKEN=%s", r.userServerAuthToken),
		fmt.Sprintf("USER_SERVER_SOCKET_PATH=%s", socketPath),
		fmt.Sprintf("ICON_PATH=%s", r.iconFileLocation()),
		fmt.Sprintf("MENU_PATH=%s", menuPath),
		fmt.Sprintf("PPID=%d", os.Getpid()),
		fmt.Sprintf("RUNNER_SERVER_URL=%s", r.runnerServer.Url()),
		fmt.Sprintf("RUNNER_SERVER_AUTH_TOKEN=%s", r.runnerServer.RegisterClient(uid)),
		fmt.Sprintf("DEBUG=%v", r.knapsack.Debug()),
		// needed for windows to find various allowed commands
		fmt.Sprintf("WINDIR=%s", os.Getenv("WINDIR")),
		"LAUNCHER_SKIP_UPDATES=true", // We already know that we want to run the version of launcher in `executablePath`, so there's no need to perform lookups
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
			r.slogger.Log(context.TODO(), slog.LevelDebug,
				scanner.Text(),
				"uid", uid,
				"subprocess", "desktop",
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
			r.slogger.Log(context.TODO(), slog.LevelError,
				"icon file did not exist, could not create it",
				"err", err,
			)
		}
	} else if err != nil {
		r.slogger.Log(context.TODO(), slog.LevelError,
			"could not check if icon file exists",
			"err", err,
		)
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

func removeFilesWithPrefix(folderPath, prefix string) error {
	return filepath.WalkDir(folderPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if !strings.HasPrefix(d.Name(), prefix) {
			return nil
		}

		// not dir, has prefix
		return os.Remove(path)
	})
}

func (r *DesktopUsersProcessesRunner) checkOsUpdate() {
	// on darwin, sometimes the desktop disappears after an OS update
	// eventhough the process is still there, so lets restart desktop
	// via killing the process and letting the runner restart it
	if runtime.GOOS != "darwin" {
		return
	}

	osVersion, err := osversion()
	if err != nil {
		r.slogger.Log(context.TODO(), slog.LevelError,
			"getting os version",
			"err", err,
		)
		return
	}

	if osVersion != r.osVersion {
		r.slogger.Log(context.TODO(), slog.LevelInfo,
			"os version changed, restarting desktop",
			"old", r.osVersion,
			"new", osVersion,
		)
		r.osVersion = osVersion
		r.killDesktopProcesses()
	}
}
