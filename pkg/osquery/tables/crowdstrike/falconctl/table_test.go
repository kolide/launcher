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
		name          string
		options       []string
		expectedLog   []string
		expectedExecs int
	}{
		{
			name: "default",
			expectedLog: []string{
				"exec-in-test",
			},
			expectedExecs: 1,
		},
		{
			name:    "allowed options as array",
			options: []string{"--aid", "--aph"},
			expectedLog: []string{
				"exec-in-test",
			},
			expectedExecs: 2,
		},
		{
			name:    "allowed options as string",
			options: []string{"--aid --aph"},
			expectedLog: []string{
				"exec-in-test",
			},
			expectedExecs: 1,
		},
		{
			name:    "disallowed option as array",
			options: []string{"--not-allowed", "--aid", "--aph"},
			expectedLog: []string{
				"exec-in-test",
				"requested option not allowed",
			},
			expectedExecs: 2,
		},
		{
			name:    "disallowed option as string",
			options: []string{"--aid --aph --not-allowed"},
			expectedLog: []string{
				"requested option not allowed",
			},
			expectedExecs: 0,
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
			require.NoError(t, err)

			for _, expectedLog := range tt.expectedLog {
				require.Contains(t, logBytes.String(), expectedLog)
			}

			// test the number of times exec was called
			require.Equal(t, tt.expectedExecs, strings.Count(logBytes.String(), "exec-in-test"))
		})
	}
}

func noopExec(_ context.Context, log log.Logger, _ int, _ []string, args []string) ([]byte, error) {
	log.Log("exec", "exec-in-test", "args", strings.Join(args, " "))
	return []byte{}, nil
}
