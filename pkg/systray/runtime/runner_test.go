//go:build darwin || linux
// +build darwin linux

package runtime

import (
	"math"
	"os"
	"os/user"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystrayUserProcessRunner_Execute(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		setup        func(*testing.T, *SystrayUsersProcessesRunner)
		assertion    func(*testing.T, *SystrayUsersProcessesRunner)
		errAssertion assert.ErrorAssertionFunc
	}{
		{
			name:         "happy path",
			errAssertion: assert.NoError,
			assertion: func(t *testing.T, r *SystrayUsersProcessesRunner) {
				user, err := user.Current()
				require.NoError(t, err)
				// make sure we have a new process
				assert.NotEmpty(t, r.uidProcs[user.Uid])
				assert.Len(t, r.uidProcs, 1)
			},
		},
		{
			name: "new process started if old one gone",
			setup: func(t *testing.T, r *SystrayUsersProcessesRunner) {
				user, err := user.Current()
				require.NoError(t, err)
				r.uidProcs[user.Uid] = &os.Process{Pid: math.MaxInt - 1}
			},
			assertion: func(t *testing.T, r *SystrayUsersProcessesRunner) {
				user, err := user.Current()
				require.NoError(t, err)
				// make sure we have a new process that doesnt match the
				assert.NotEqual(t, r.uidProcs[user.Uid].Pid, math.MaxInt-1)
				assert.Len(t, r.uidProcs, 1)
			},
			errAssertion: assert.NoError,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := New(log.NewNopLogger(), time.Millisecond*100)

			if tt.setup != nil {
				tt.setup(t, r)
			}

			go func() {
				tt.errAssertion(t, r.Execute())
			}()

			// let is run a few interval
			time.Sleep(r.executionInterval * 3)
			r.Interrupt(nil)

			if tt.assertion != nil {
				tt.assertion(t, r)
			}
		})
	}
}
