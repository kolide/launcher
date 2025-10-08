//nolint:paralleltest
package runsimple

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/kolide/launcher/pkg/osquery/testutil"
	"github.com/stretchr/testify/require"
)

var testOsqueryBinary string

// TestMain overrides the default test main function. This allows us to share setup/teardown.
func TestMain(m *testing.M) {
	testOsqueryBinary, _ = testutil.DownloadOsqueryOrDie("stable")

	// Run the tests!
	retCode := m.Run()

	os.Exit(retCode) //nolint:forbidigo // Fine to use os.Exit in tests
}

func Test_OsqueryRunSqlNoIO(t *testing.T) {
	osq, err := NewOsqueryProcess(testOsqueryBinary)
	require.NoError(t, err)

	require.NoError(t, osq.RunSql(t.Context(), []byte("select 1")))
}

func Test_OsqueryRunSql(t *testing.T) {
	tests := []struct {
		name      string
		sql       string
		expectErr bool
		contains  []string
	}{
		{
			name:      "Bad SQL",
			sql:       "this is not sql;",
			expectErr: true,
		},
		// osquery behavior is quite inconsistent around this stuff. So several tests
		// are commented out.
		// https://github.com/osquery/osquery/issues/8148
		// {
		// 	name:      "Bad SQL, no semicolon,
		// 	sql:       "this is not sql, no semicolon",
		// 	expectErr: true,
		// },
		//
		// {
		// 	name: "select 1",
		// 	sql:  "select 1",
		// },
		{
			name:     "select 1;",
			sql:      "select 1;",
			contains: []string{"1"},
		},
		{
			name:     "multiselect",
			sql:      "select 1; select 2;",
			contains: []string{"1", "2"},
		},
		{
			name:     "comments",
			sql:      "select 1; select 2; \n--this is a comment\nselect 3;",
			contains: []string{"1", "2", "3"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// No parallel, to many execs

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			osq, err := NewOsqueryProcess(
				testOsqueryBinary,
				WithStdout(&stdout),
				WithStderr(&stderr),
			)
			require.NoError(t, err)

			if tt.expectErr {
				require.Error(t, osq.RunSql(t.Context(), []byte(tt.sql)))
				require.Contains(t, stderr.String(), "Error")
				return
			}

			require.NoError(t, osq.RunSql(t.Context(), []byte(tt.sql)))

			for _, s := range tt.contains {
				require.Contains(t, stdout.String(), s, "Output should contain %s", s)
			}
			{
				_, err := decodeJsonL(&stdout)
				require.NoError(t, err)
			}
		})
	}

}

func decodeJsonL(data io.Reader) ([]any, error) {
	var result []any
	decoder := json.NewDecoder(data)

	count := 0
	for {
		var object any

		switch err := decoder.Decode(&object); err {
		case nil:
			result = append(result, object)
		case io.EOF:
			return result, nil
		default:
			return nil, fmt.Errorf("unmarshalling jsonl: %w", err)
		}

		count += 1

		if count > 50 {
			return nil, errors.New("stuck in a loop. Count exceeds 50")
		}
	}
}
