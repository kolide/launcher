//go:build !windows
// +build !windows

package tablehelpers

import (
	"context"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
)

func TestExec(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name    string
		timeout int
		bins    []string
		args    []string
		err     bool
	}{
		{
			name: "no binaries",
			bins: []string{"/hello/world", "/hello/friends"},
			err:  true,
		},
		{
			name: "eventually finds binary",
			bins: []string{"/hello/world", "/bin/ps", "/usr/bin/ps"},
		},
	}

	ctx := context.Background()
	logger := log.NewNopLogger()

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.timeout == 0 {
				tt.timeout = 30
			}
			output, err := Exec(ctx, logger, tt.timeout, tt.bins, tt.args, false)
			if tt.err {
				assert.Error(t, err)
				assert.Empty(t, output)
			} else {
				assert.NoError(t, err)
				assert.Less(t, 0, len(output))
			}
		})
	}
}
