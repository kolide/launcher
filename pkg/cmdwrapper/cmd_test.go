package cmdwrapper

import (
	"context"
	"os/user"
	"testing"

	"github.com/magiconair/properties/assert"
	"github.com/stretchr/testify/require"
)

func TestCmd(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	currentUser, err := user.Current()
	if err != nil {
		panic(err)
	}

	_ = currentUser

	var tests = []struct {
		name       string // test name, optional, for debugging errors
		arg0       string
		args       []string
		opts       []Option
		rootNeeded bool
		stdout     string
		stderr     string
		err        error
	}{
		{
			arg0: "true",
		},
		{
			name:   "simple whoami",
			arg0:   "whoami",
			stdout: currentUser.Username,
		},
		{
			name:   "runas current user",
			arg0:   "whoami",
			stdout: currentUser.Username,
			opts:   []Option{RunAsUser(currentUser.Username)},
		},
		{
			name:       "forced runas currentuser",
			arg0:       "whoami",
			rootNeeded: true,
			stdout:     currentUser.Username,
			opts:       []Option{RunAsUser(currentUser.Username), AlwaysRunAsUser()},
		},
		{
			name:       "forced runas currentuser",
			arg0:       "whoami",
			rootNeeded: true,
			stdout:     "nobody",
			opts:       []Option{RunAsUser("nobody")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.rootNeeded && currentUser.Uid != "0" {
				t.Skip("Not root, can't test. Currently ", currentUser.Username)
			}

			stdout, stderr, err := Run(ctx, tt.arg0, tt.args, tt.opts...)

			if tt.err != nil {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			assert.Equal(t, stdout, tt.stdout, "stdout")
			assert.Equal(t, stderr, tt.stderr, "stderr")
		})
	}
}
