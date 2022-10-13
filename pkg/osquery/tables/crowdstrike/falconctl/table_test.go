package falconctl

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/stretchr/testify/require"
)

// TestOptionRestrictions tests that the table only allows the options we expect.
func TestOptionRestrictions(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name        string
		options     []string
		expectedErr bool
	}{
		{
			name: "default",
		},
		{
			name:    "allowed options as array",
			options: []string{"--aid", "--aph"},
		},
		{
			name:    "allowed options as string",
			options: []string{"--aid --aph"},
		},
		{
			name:        "disallowed option as array",
			options:     []string{"--not-allowed", "--aid", "--aph"},
			expectedErr: true,
		},
		{
			name:        "disallowed option as string",
			options:     []string{"--aid --aph --not-allowed"},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var logBytes bytes.Buffer

			testTable := &falconctlOptionsTable{
				logger:   log.NewLogfmtLogger(&logBytes),
				execFunc: noopExec,
			}

			mockQC := tablehelpers.MockQueryContext(map[string][]string{
				"options": tt.options,
			})

			// We've overridden exec with a noop, but we can check that we get the expected error back
			_, err := testTable.generate(context.TODO(), mockQC)

			if tt.expectedErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "requested option not allowed")
				require.Equal(t, 0, strings.Count(logBytes.String(), "exec-in-test"))
				return
			}

			require.NoError(t, err)

			// We normally expect an exec per option. But the default is special.
			if tt.name == "default" {
				require.Equal(t, 1, strings.Count(logBytes.String(), "exec-in-test"))
			} else {
				require.Equal(t, len(tt.options), strings.Count(logBytes.String(), "exec-in-test"))
			}

			for _, option := range tt.options {
				require.Contains(t, logBytes.String(), option)
			}
		})
	}
}

func noopExec(_ context.Context, log log.Logger, _ int, _ []string, args []string) ([]byte, error) {
	log.Log("exec", "exec-in-test", "args", strings.Join(args, " "))
	return []byte{}, nil
}
