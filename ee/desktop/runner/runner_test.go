package runner

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/consoleuser"
	"github.com/kolide/launcher/ee/desktop/user/notify"
	"github.com/kolide/launcher/ee/presencedetection"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDesktopUserProcessRunner_Execute(t *testing.T) {
	t.Parallel()

	// When running this using the golang test harness, it will leave behind process if you do not build the binary first.
	// On mac os you can find these by setting the executable path to an empty string before running the tests, then search
	// the processes in a terminal using: ps aux -o ppid | runtime.test after the tests have completed, you'll also see the
	// CPU consumption go way up.

	// To get around the issue mentioned above, build the binary first and set its path as the executable path on the runner.
	executablePath := filepath.Join(t.TempDir(), "desktop-test", "launcher")

	if runtime.GOOS == "windows" {
		executablePath = fmt.Sprintf("%s.exe", executablePath)
	}

	// due to flakey tests we are tracking the time it takes to build and attempting emit a meaningful error if we time out
	timeout := time.Minute * 2
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()

	// We may already have a built binary available -- check for that first
	if err := symlinkPreexistingBinary(ctx, executablePath); err != nil {
		// No binary available -- build one instead
		cmd := exec.CommandContext(ctx, "go", "build", "-o", executablePath, "../../../cmd/launcher") //nolint:forbidigo // Fine to use exec.CommandContext in test
		buildStartTime := time.Now()
		out, err := cmd.CombinedOutput()
		if err != nil {
			err = fmt.Errorf("building launcher binary for desktop testing: %w", err)

			if time.Since(buildStartTime) >= timeout {
				err = fmt.Errorf("timeout (%v) met: %w", timeout, err)
			}
		}
		require.NoError(t, err, string(out))
	}

	tests := []struct {
		name          string
		setup         func(*testing.T, *DesktopUsersProcessesRunner)
		logContains   []string
		cleanShutdown bool
	}{
		{
			name: "happy path",
			logContains: []string{
				"desktop started",
				"interrupt received, exiting desktop execute loop",
				"all desktop processes shutdown successfully",
			},
			cleanShutdown: true,
		},
		{
			name: "new process started if old one gone",
			setup: func(t *testing.T, r *DesktopUsersProcessesRunner) {
				// in the current CI environment (GitHub Actions) the linux runner
				// does not have a console user, so we don't expect any processes
				// to be started.
				if os.Getenv("CI") == "true" && runtime.GOOS == "linux" {
					return
				}
				user, err := user.Current()
				require.NoError(t, err)
				r.uidProcs[user.Uid] = processRecord{
					Process: &os.Process{},
					path:    "test",
				}
			},
			logContains: []string{
				"found existing desktop process dead for console user",
				"interrupt received, exiting desktop execute loop",
				"all desktop processes shutdown successfully",
			},
			cleanShutdown: true,
		},
		{
			name: "procs waitgroup times out",
			setup: func(t *testing.T, r *DesktopUsersProcessesRunner) {
				r.interruptTimeout = time.Millisecond
				// wg will never be done, so we should time out
				r.procsWg.Add(1)
			},
			logContains: []string{
				"timeout waiting for desktop processes to exit",
				"interrupt received, exiting desktop execute loop",
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var logBytes threadsafebuffer.ThreadSafeBuffer
			slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			}))

			mockKnapsack := mocks.NewKnapsack(t)
			mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.DesktopEnabled)
			mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.DesktopGoMaxProcs)
			mockKnapsack.On("DesktopUpdateInterval").Return(time.Millisecond * 250)
			mockKnapsack.On("DesktopMenuRefreshInterval").Return(time.Millisecond * 250)
			mockKnapsack.On("DesktopGoMaxProcs").Return(2).Maybe()
			mockKnapsack.On("KolideServerURL").Return("somewhere-over-the-rainbow.example.com")

			// if we're not in CI, always expect desktop enabled call
			// if we are in CI only expect desktop enabled on windows and darwin
			// since linux CI has no desktop user to make desktop process for
			mockKnapsack.On("DesktopEnabled").Return(true).Maybe()

			mockKnapsack.On("Slogger").Return(slogger)
			mockKnapsack.On("InModernStandby").Return(false)
			mockKnapsack.On("SystrayRestartEnabled").Return(false).Maybe()

			if os.Getenv("CI") != "true" || runtime.GOOS != "linux" {
				// Only expect that we call Debug (to set the DEBUG flag on the process) if we actually expect
				// to be starting a process.
				mockKnapsack.On("Debug").Return(true)
			}

			r, err := New(
				mockKnapsack,
				nil,
				WithExecutablePath(executablePath),
				WithInterruptTimeout(time.Second*5),
				WithAuthToken("test-auth-token"),
				WithUsersFilesRoot(launcherRootDir(t)),
			)
			require.NoError(t, err)

			if tt.setup != nil {
				tt.setup(t, r)
			}

			go func() {
				assert.NoError(t, r.Execute())
			}()

			// let it run a few intervals
			time.Sleep(r.updateInterval * 6)
			interruptStart := time.Now()
			r.Interrupt(nil)

			user, err := user.Current()
			require.NoError(t, err)

			// in the current CI environment (GitHub Actions) the linux runner
			// does not have a console user, so we don't expect any processes
			// to be started.
			if tt.cleanShutdown || (os.Getenv("CI") == "true" && runtime.GOOS == "linux") {
				require.NoError(t, backoff.WaitFor(func() error {
					if len(r.uidProcs) == 0 {
						return nil
					}

					return fmt.Errorf("expected no processes, found %d", len(r.uidProcs))
				}, 30*time.Second, 1*time.Second))
			} else {
				if runtime.GOOS == "windows" {
					currentUids, err := consoleuser.CurrentUids(ctx)
					require.NoError(t, err)
					require.Equal(t, 1, len(currentUids))
					assert.Contains(t, r.uidProcs, currentUids[0], "process not found for expected user, logs: ", logBytes.String())
				} else {
					assert.Contains(t, r.uidProcs, user.Uid)
				}
				assert.Len(t, r.uidProcs, 1)

				if len(tt.logContains) > 0 {
					for _, s := range tt.logContains {
						assert.Contains(t, logBytes.String(), s)
					}
				}
			}

			t.Cleanup(func() {
				// the cleanup of the t.TempDir() will happen before the binary built for the tests is closed,
				// on windows this will cause an error, so just wait for all the processes to finish
				for _, p := range r.uidProcs {
					if !r.processExists(p) {
						continue
					}
					// intentionally ignoring the error here
					// CI will intermittently fail with "wait: no child processes" due runner.go also calling process.Wait()
					// racing with this code to remove the child process
					p.Process.Wait()
				}
			})

			// Confirm we can call Interrupt multiple times without blocking
			interruptComplete := make(chan struct{})
			expectedInterrupts := 3
			for i := 0; i < expectedInterrupts; i += 1 {
				go func() {
					r.Interrupt(nil)
					interruptComplete <- struct{}{}
				}()
			}

			receivedInterrupts := 0
			for {
				if receivedInterrupts >= expectedInterrupts {
					break
				}

				select {
				case <-interruptComplete:
					receivedInterrupts += 1
					continue
				case <-time.After(5 * time.Second):
					t.Errorf("could not call interrupt multiple times and return within 5 seconds -- interrupted at %s, received %d interrupts before timeout; logs: \n%s\n", interruptStart.String(), receivedInterrupts, logBytes.String())
					t.FailNow()
				}
			}

			require.Equal(t, expectedInterrupts, receivedInterrupts)
		})
	}
}

func symlinkPreexistingBinary(ctx context.Context, executablePath string) error {
	builtBinaryPath := filepath.Join("..", "..", "..", "build", "launcher")
	if runtime.GOOS == "windows" {
		builtBinaryPath += "launcher.exe"
	}
	absPath, err := filepath.Abs(builtBinaryPath)
	if err != nil {
		return fmt.Errorf("getting absolute path for %s: %w", builtBinaryPath, err)
	}
	builtBinaryPath = filepath.Clean(absPath)

	// See if file exists
	if _, err := os.Stat(builtBinaryPath); os.IsNotExist(err) {
		return fmt.Errorf("no preexisting binary at %s", builtBinaryPath)
	}

	// Get our current version
	gitCmd := exec.CommandContext(ctx, "git", "describe", "--tags", "--always", "--dirty") //nolint:forbidigo // Fine to use exec.CommandContext in test
	versionOut, err := gitCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("getting current version: %w", err)
	}
	currentVersion := strings.TrimPrefix(strings.TrimSpace(string(versionOut)), "v")

	// Binary exists -- see if the version is a match
	cmd := exec.CommandContext(ctx, builtBinaryPath, "-version") //nolint:forbidigo // Fine to use exec.CommandContext in test
	cmd.Env = append(cmd.Environ(), "LAUNCHER_SKIP_UPDATES=TRUE")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("checking version: %w", err)
	}

	lines := strings.Split(string(out), "\n")
	binaryVersion := ""
	for _, line := range lines {
		if !strings.HasPrefix(line, "launcher - version") {
			continue
		}

		// We found the version
		binaryVersion = strings.TrimSpace(strings.TrimPrefix(line, "launcher - version"))
		break
	}

	if binaryVersion != currentVersion {
		return fmt.Errorf("built version %s does not match current version %s", binaryVersion, currentVersion)
	}

	if err := os.MkdirAll(filepath.Dir(executablePath), 0755); err != nil {
		return fmt.Errorf("making test dir: %w", err)
	}

	return os.Symlink(builtBinaryPath, executablePath)
}

func launcherRootDir(t *testing.T) string {
	safeTestName := fmt.Sprintf("%s_%s", "launcher_desktop_test", ulid.New())

	path := filepath.Join(t.TempDir(), safeTestName)

	if runtime.GOOS != "windows" {
		path = filepath.Join("/tmp", safeTestName)
	}

	require.NoError(t, os.MkdirAll(path, 0700))

	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(path))
	})

	return path
}

func Test_writeIconPath(t *testing.T) {
	t.Parallel()

	// Create a temp directory to use as our root directory
	rootDir := t.TempDir()

	// Create runner for test
	r := DesktopUsersProcessesRunner{
		usersFilesRoot: rootDir,
	}

	// Test that if the icon doesn't exist in the root dir, the runner will create it.
	r.writeIconFile()
	_, err := os.Stat(filepath.Join(rootDir, iconFilename()))
	require.NoError(t, err, "icon file not created")
}

func TestUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    io.Reader
		err      bool
		contains []string
	}{
		{
			name:     "works",
			input:    strings.NewReader(`{"items":[{"label":"one","tooltip":null,"action":null,"items":[],"disabled":true,"separator":false},{"label":null,"tooltip":null,"action":null,"items":[],"disabled":true,"separator":true},{"label":"two","tooltip":null,"action":null,"items":[],"disabled":true,"separator":false}],"icon":null,"tooltip":"Kolide"}`),
			contains: []string{"Kolide", "one", "two"},
		},
		{
			name:  "empty",
			input: strings.NewReader(""),
		},
		{
			name:  "nil",
			input: nil,
			err:   true,
		},
		{
			name:  "bad json",
			input: strings.NewReader("hello"),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockKnapsack := mocks.NewKnapsack(t)
			mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.DesktopEnabled)
			mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.DesktopGoMaxProcs)
			mockKnapsack.On("DesktopUpdateInterval").Return(time.Millisecond * 250)
			mockKnapsack.On("DesktopMenuRefreshInterval").Return(time.Millisecond * 250)
			mockKnapsack.On("DesktopGoMaxProcs").Return(2).Maybe()
			mockKnapsack.On("KolideServerURL").Return("somewhere-over-the-rainbow.example.com")
			mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
			mockKnapsack.On("InModernStandby").Return(false)

			dir := t.TempDir()
			r, err := New(mockKnapsack, nil, WithUsersFilesRoot(dir))
			require.NoError(t, err)

			if tt.err {
				require.Error(t, r.Update(tt.input))
				return
			}
			require.NoError(t, r.Update(tt.input))

			menuFH, err := os.Open(r.menuPath())
			require.NoError(t, err)
			defer menuFH.Close()

			menuContents, err := io.ReadAll(menuFH)
			require.NoError(t, err)
			for _, str := range tt.contains {
				assert.Contains(t, string(menuContents), str)
			}
		})
	}
}

func TestSendNotification_NoProcessesYet(t *testing.T) {
	t.Parallel()

	mockKnapsack := mocks.NewKnapsack(t)
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.DesktopEnabled)
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.DesktopGoMaxProcs)
	mockKnapsack.On("DesktopUpdateInterval").Return(time.Millisecond * 250)
	mockKnapsack.On("DesktopMenuRefreshInterval").Return(time.Millisecond * 250)
	mockKnapsack.On("DesktopGoMaxProcs").Return(2).Maybe()
	mockKnapsack.On("KolideServerURL").Return("somewhere-over-the-rainbow.example.com")
	mockKnapsack.On("DesktopEnabled").Return(true)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
	mockKnapsack.On("InModernStandby").Return(false)

	dir := t.TempDir()
	r, err := New(mockKnapsack, nil, WithUsersFilesRoot(dir))
	require.NoError(t, err)

	require.Equal(t, 0, len(r.uidProcs))
	err = r.SendNotification(notify.Notification{Title: "test", Body: "test"})
	require.Error(t, err, "should not be able to send notification when there are no child processes")
}

func TestDesktopUsersProcessesRunner_setupSocketPath(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("windows sockets differently, test does not apply")
	}

	runner := DesktopUsersProcessesRunner{}

	u, err := user.Current()
	require.NoError(t, err)

	socketPath, err := runner.setupSocketPath(u.Uid)
	require.NoError(t, err)

	// get dir of socket path
	socketDir := filepath.Dir(socketPath)

	shouldNotBeDeletedFilePath := filepath.Join(socketDir, "dont_delete_me")
	require.NoError(t, os.WriteFile(shouldNotBeDeletedFilePath, []byte{}, 0600))

	// create some fake socket files to get cleaned up
	for i := 0; i < 3; i++ {
		socketPath := filepath.Join(socketDir, fmt.Sprintf("%s_%d", nonWindowsDesktopSocketPrefix, i))
		require.NoError(t, os.WriteFile(socketPath, []byte{}, 0600))
	}

	// sanity check that files got created
	count, err := countFilesWithPrefix(socketDir, "")
	require.NoError(t, err)
	require.Equal(t, 4, count)

	// calling set up socket path should remove the fake socket files
	_, err = runner.setupSocketPath(u.Uid)
	require.NoError(t, err)

	// make sure all old sockets got deleted
	count, err = countFilesWithPrefix(socketDir, nonWindowsDesktopSocketPrefix)
	require.NoError(t, err)
	require.Zero(t, count)

	// make sure non socket file did not get deleted
	count, err = countFilesWithPrefix(socketDir, "")
	require.NoError(t, err)
	require.Equal(t, 1, count)

	// clean up the extra file
	require.NoError(t, os.Remove(shouldNotBeDeletedFilePath))
}

func countFilesWithPrefix(folderPath, prefix string) (int, error) {
	count := 0

	if err := filepath.WalkDir(folderPath, func(path string, d os.DirEntry, err error) error {
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
		count++
		return nil
	}); err != nil {
		return 0, err
	}

	return count, nil
}

func TestDesktopUsersProcessesRunner_DetectPresence(t *testing.T) {
	t.Parallel()

	t.Run("no user procs", func(t *testing.T) {
		t.Parallel()

		runner := DesktopUsersProcessesRunner{}
		d, err := runner.DetectPresence("whatevs", time.Second)
		require.Error(t, err)
		require.Equal(t, presencedetection.DetectionFailedDurationValue, d)
	})

	t.Run("cant connect to user server", func(t *testing.T) {
		t.Parallel()

		u, err := user.Current()
		require.NoError(t, err)

		runner := DesktopUsersProcessesRunner{
			uidProcs: map[string]processRecord{
				u.Uid: {},
			},
		}

		d, err := runner.DetectPresence("whatevs", time.Second)
		require.Error(t, err)
		require.Equal(t, presencedetection.DetectionFailedDurationValue, d)
	})
}
