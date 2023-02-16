package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDesktopUserProcessRunner_Execute(t *testing.T) {
	t.Parallel()

	// When running this using the golang test harness, it will leave behind process if you do not build the binary first.
	// On mac os you can find these by setting the executable path to an empty string before running the tests, then search
	// the processes in a terminal using: ps aux -o ppid | runtime.test after the tests have completed, you'll also see the
	// CPU consumption go way up.

	// To get around the issue mentioned above, build the binary first and set its path as the executable path on the runner.
	executablePath := filepath.Join(t.TempDir(), "desktop-test")

	if runtime.GOOS == "windows" {
		executablePath = fmt.Sprintf("%s.exe", executablePath)
	}

	// due to flakey tests we are tracking the time it takes to build and attempting emit a meaningful error if we time out
	timeout := time.Second * 60
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", executablePath, "../../../cmd/launcher")
	buildStartTime := time.Now()
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("building launcher binary for desktop testing: %w", err)

		if time.Since(buildStartTime) >= timeout {
			err = fmt.Errorf("timeout (%v) met: %w", timeout, err)
		}
	}
	require.NoError(t, err, string(out))

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
					process: &os.Process{},
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

			var logBytes threadSafeBuffer

			r := New(
				WithLogger(log.NewLogfmtLogger(&logBytes)),
				WithExecutablePath(executablePath),
				WithHostname("somewhere-over-the-rainbow.example.com"),
				WithUpdateInterval(time.Millisecond*250),
				WithInterruptTimeout(time.Second*5),
				WithAuthToken("test-auth-token"),
				WithUsersFilesRoot(launcherRootDir(t)),
				WithProcessSpawningEnabled(true),
			)

			if tt.setup != nil {
				tt.setup(t, r)
			}

			go func() {
				assert.NoError(t, r.Execute())
			}()

			// let is run a few interval
			time.Sleep(r.updateInterval * 3)
			r.Interrupt(nil)

			user, err := user.Current()
			require.NoError(t, err)

			// in the current CI environment (GitHub Actions) the linux runner
			// does not have a console user, so we don't expect any processes
			// to be started.
			if tt.cleanShutdown || (os.Getenv("CI") == "true" && runtime.GOOS == "linux") {
				assert.Len(t, r.uidProcs, 0, "unexpected process: logs: %s", logBytes.String())
			} else {
				assert.Contains(t, r.uidProcs, user.Uid)
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
					if processExists(p) {
						state, err := p.process.Wait()
						require.NoError(t, err, fmt.Sprintf("failed to wait for process: %v", state))
					}
				}
			})
		})
	}
}

// thank you zupa https://stackoverflow.com/a/36226525
type threadSafeBuffer struct {
	b bytes.Buffer
	m sync.Mutex
}

func (b *threadSafeBuffer) Read(p []byte) (n int, err error) {
	b.m.Lock()
	defer b.m.Unlock()
	return b.b.Read(p)
}

func (b *threadSafeBuffer) Write(p []byte) (n int, err error) {
	b.m.Lock()
	defer b.m.Unlock()
	return b.b.Write(p)
}

func (b *threadSafeBuffer) String() string {
	b.m.Lock()
	defer b.m.Unlock()
	return b.b.String()
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
