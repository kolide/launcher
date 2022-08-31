package runtime

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
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

	// When running this using the golang test harness, it will leave behind proccess if you do not build the binary first.
	// On mac os you can find these by setting the executable path to an empty string before running the tests, then search
	// the processes in a terminal using: ps aux -o ppid | runtime.test after the tests have completed, you'll also see the
	// CPU consumtion go way up.

	// To get around the issue mentioned above, build the binary first and set it's path as the executable path on the runner.
	executablePath := filepath.Join(t.TempDir(), "desktop-test")

	if runtime.GOOS == "windows" {
		executablePath = fmt.Sprintf("%s.exe", executablePath)
	}

	err := exec.Command("go", "build", "-o", executablePath, "../../../cmd/launcher").Run()
	require.NoError(t, err)

	tests := []struct {
		name  string
		setup func(*testing.T, *DesktopUsersProcessesRunner)
	}{
		{
			name: "happy path",
		},
		{
			name: "new process started if old one gone",
			setup: func(t *testing.T, r *DesktopUsersProcessesRunner) {
				user, err := user.Current()
				require.NoError(t, err)
				// linter complains about math.MaxInt, but it's wrong, math.MaxInt exists
				r.uidProcs[user.Uid] = &os.Process{Pid: math.MaxInt - 1} //nolint:typecheck
			},
		},
		{
			name: "procs waitgroup times out",
			setup: func(t *testing.T, r *DesktopUsersProcessesRunner) {
				r.procsWgTimeout = time.Millisecond
				// wg will never be done, so we should time out
				r.procsWg.Add(1)
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := New(log.NewNopLogger(), time.Second*1)
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

			t.Cleanup(func() {
				// the cleanup of the t.TempDir() will happen before the binary built for the tests is closed,
				// on windows this will cause an error, so just wait for all the processes to finish
				for _, p := range r.uidProcs {
					p.Wait()
				}
			})
		})
	}
}
