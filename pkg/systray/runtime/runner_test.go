//go:build darwin
// +build darwin

package runtime

import (
	"math"
	"os"
	"os/user"
	"syscall"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystrayUserProcessRunner_Execute(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup func(*testing.T, *SystrayUsersProcessesRunner)
	}{
		{
			name: "happy path",
		},
		{
			name: "new process started if old one gone",
			setup: func(t *testing.T, r *SystrayUsersProcessesRunner) {
				user, err := user.Current()
				require.NoError(t, err)
				// linter complains about math.MaxInt, but it's wrong, math.MaxInt exists
				// nolint: typecheck
				r.uidProcs[user.Uid] = &os.Process{Pid: math.MaxInt - 1}
			},
		},
		{
			name: "procs waitgroup times out",
			setup: func(t *testing.T, r *SystrayUsersProcessesRunner) {
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

			if tt.setup != nil {
				tt.setup(t, r)
			}

			executeDone := make(chan struct{})
			go func() {
				assert.NoError(t, r.Execute())
				executeDone <- struct{}{}
			}()

			// let is run a few interval
			time.Sleep(r.executionInterval * 3)
			r.Interrupt(nil)

			<-executeDone

			user, err := user.Current()
			require.NoError(t, err)
			assert.Contains(t, r.uidProcs, user.Uid)
			assert.Len(t, r.uidProcs, 1)

			t.Cleanup(func() {
				for _, proc := range r.uidProcs {
					proc.Signal(syscall.SIGTERM)
				}

				<-time.After(time.Second)

				// make sure we clean up an remaining processes
				for _, proc := range r.uidProcs {
					proc.Kill()
					proc.Wait()
				}
			})
		})
	}
}
