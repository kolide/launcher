//go:build !windows
// +build !windows

package tablehelpers

import (
	"context"
	"testing"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
)

func TestExec(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name    string
		timeout int
		bin     allowedcmd.AllowedCommand
		args    []string
		err     bool
		output  string
	}{
		{
			name:   "output",
			bin:    allowedcmd.Echo,
			args:   []string{"hello"},
			output: "hello\n",
		},
	}

	ctx := context.Background()
	slogger := multislogger.New().Logger

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.timeout == 0 {
				tt.timeout = 30
			}
			output, err := Exec(ctx, slogger, tt.timeout, tt.bin, tt.args, false)
			if tt.err {
				assert.Error(t, err)
				assert.Empty(t, output)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, []byte(tt.output), output)
			}
		})
	}
}
