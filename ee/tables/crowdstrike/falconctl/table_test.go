//go:build !windows
// +build !windows

package falconctl

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

// TestOptionRestrictions tests that the table only allows the options we expect.
func TestOptionRestrictions(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name              string
		options           []string
		expectedExecs     int
		expectedDisallows int
	}{
		{
			name:              "default",
			expectedExecs:     1,
			expectedDisallows: 0,
		},
		{
			name:              "allowed options as array",
			options:           []string{"--aid", "--aph"},
			expectedExecs:     2,
			expectedDisallows: 0,
		},
		{
			name:              "allowed options as string",
			options:           []string{"--aid --aph"},
			expectedExecs:     1,
			expectedDisallows: 0,
		},
		{
			name:              "disallowed option as array",
			options:           []string{"--not-allowed", "--definitely-not-allowed", "--aid", "--aph"},
			expectedExecs:     2,
			expectedDisallows: 2,
		},
		{
			name:              "disallowed option as string",
			options:           []string{"--aid --aph --not-allowed"},
			expectedExecs:     0,
			expectedDisallows: 1,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var logBytes threadsafebuffer.ThreadSafeBuffer

			testTable := &falconctlOptionsTable{
				slogger:  multislogger.New(slog.NewJSONHandler(&logBytes, &slog.HandlerOptions{Level: slog.LevelDebug})).Logger,
				execFunc: noopExec,
			}

			mockQC := tablehelpers.MockQueryContext(map[string][]string{
				"options": tt.options,
			})

			_, err := testTable.generate(context.TODO(), mockQC)
			require.NoError(t, err)

			// test the number of times exec was called
			require.Equal(t, tt.expectedExecs, strings.Count(logBytes.String(), "exec-in-test"))

			// test the number of times we disallowed an option
			require.Equal(t, tt.expectedDisallows, strings.Count(logBytes.String(), "requested option not allowed"))
		})
	}
}

func noopExec(ctx context.Context, slogger *slog.Logger, _ int, _ allowedcmd.AllowedCommand, args []string, _ ...tablehelpers.ExecOps) ([]byte, error) {
	slogger.Log(ctx, slog.LevelInfo, "exec-in-test", "args", strings.Join(args, " "))
	return []byte{}, nil
}
