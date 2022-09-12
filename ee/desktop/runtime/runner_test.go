package runtime

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDesktopUserProcessRunner_Execute(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "linux" {
		t.Skip("skipping linux test because it's not implemented")
	}

	// When running this using the golang test harness, it will leave behind process if you do not build the binary first.
	// On mac os you can find these by setting the executable path to an empty string before running the tests, then search
	// the processes in a terminal using: ps aux -o ppid | runtime.test after the tests have completed, you'll also see the
	// CPU consumption go way up.

	// To get around the issue mentioned above, build the binary first and set it's path as the executable path on the runner.
	executablePath := filepath.Join(t.TempDir(), "desktop-test")

	if runtime.GOOS == "windows" {
		executablePath = fmt.Sprintf("%s.exe", executablePath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", executablePath, "../../../cmd/launcher")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	tests := []struct {
		name        string
		setup       func(*testing.T, *DesktopUsersProcessesRunner)
		logContains []string
	}{
		{
			name: "happy path",
			logContains: []string{
				"desktop started",
				"interrupt received, exiting desktop execute loop",
				"all desktop processes shutdown successfully",
			},
		},
		{
			name: "new process started if old one gone",
			setup: func(t *testing.T, r *DesktopUsersProcessesRunner) {
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
		},
		{
			name: "procs waitgroup times out",
			setup: func(t *testing.T, r *DesktopUsersProcessesRunner) {
				r.procsWgTimeout = time.Millisecond
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

			r := New(log.NewLogfmtLogger(&logBytes), time.Second*1, "some-where-over-the-rainbow.com")
			r.executablePath = executablePath

			if tt.setup != nil {
				tt.setup(t, r)
			}

			go func() {
				assert.NoError(t, r.Execute())
			}()

			// let is run a few interval
			time.Sleep(r.executionInterval * 3)
			r.Interrupt(nil)

			user, err := user.Current()
			require.NoError(t, err)
			assert.Contains(t, r.uidProcs, user.Uid)
			assert.Len(t, r.uidProcs, 1)

			if len(tt.logContains) > 0 {
				for _, s := range tt.logContains {
					assert.Contains(t, logBytes.String(), s)
				}
			}

			t.Cleanup(func() {
				// the cleanup of the t.TempDir() will happen before the binary built for the tests is closed,
				// on windows this will cause an error, so just wait for all the processes to finish
				for _, p := range r.uidProcs {
					if processExists(p) {
						p.process.Wait()
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
