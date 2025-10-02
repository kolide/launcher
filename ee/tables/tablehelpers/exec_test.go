//go:build !windows
// +build !windows

package tablehelpers

import (
	"bytes"
	"testing"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		timeoutSeconds int
		execCmd        allowedcmd.AllowedCommand
		args           []string
		wantStdout     bool
		wantStderr     bool
		opts           []ExecOps
		assertion      assert.ErrorAssertionFunc
	}{
		{
			name:           "happy path",
			timeoutSeconds: 1,
			execCmd:        allowedcmd.Echo,
			args:           []string{"hi"},
			wantStdout:     true,
			wantStderr:     false,
			assertion:      assert.NoError,
		},
		{
			name:           "timeout",
			timeoutSeconds: 0,
			execCmd:        allowedcmd.Echo,
			args:           []string{"hi"},
			wantStdout:     false,
			wantStderr:     false,
			assertion:      assert.Error,
		},
		{
			name:           "stderr",
			timeoutSeconds: 1,
			execCmd:        allowedcmd.Ps,
			args:           []string{"-z"},
			wantStdout:     false,
			wantStderr:     true,
			assertion:      assert.Error,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			tt.assertion(t, Run(t.Context(), multislogger.NewNopLogger(), tt.timeoutSeconds, tt.execCmd, tt.args, stdout, stderr))

			if tt.wantStdout {
				require.NotEmpty(t, stdout.String())
			} else {
				require.Empty(t, stdout.String())
			}

			if tt.wantStderr {
				require.NotEmpty(t, stderr.String())
			} else {
				require.Empty(t, stderr.String())
			}
		})
	}
}

func TestRunSimple(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		timeoutSeconds int
		cmd            allowedcmd.AllowedCommand
		args           []string
		opts           []ExecOps
		want           []byte
		assertion      assert.ErrorAssertionFunc
	}{
		{
			name:           "happy path",
			timeoutSeconds: 1,
			cmd:            allowedcmd.Echo,
			args:           []string{"hi"},
			want:           []byte("hi\n"),
			assertion:      assert.NoError,
		},
		{
			name:           "error",
			timeoutSeconds: 1,
			cmd:            allowedcmd.Ps,
			args:           []string{"-z"},
			want:           nil,
			assertion:      assert.Error,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := RunSimple(t.Context(), multislogger.NewNopLogger(), tt.timeoutSeconds, tt.cmd, tt.args, tt.opts...)
			tt.assertion(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
