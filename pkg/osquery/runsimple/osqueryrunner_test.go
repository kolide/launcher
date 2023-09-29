//nolint:paralleltest
package runsimple

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
)

func Test_OsquerySingleSqlNoIO(t *testing.T) {
	osquerydPath := os.Getenv("OSQUERYD_PATH")

	if osquerydPath == "" {
		t.Skip("No osquery. Not implemented")
	}

	osq, err := NewOsqueryProcess(osquerydPath)
	require.NoError(t, err)

	require.NoError(t, osq.RunSql(context.TODO(), []byte("select 1")))
}

func Test_OsquerySingleSqlErr(t *testing.T) {
	osquerydPath := os.Getenv("OSQUERYD_PATH")

	if osquerydPath == "" {
		t.Skip("No osquery. Not implemented")
	}

	tests := []struct {
		name      string
		sql       string
		expectErr bool
		jsonMulti bool
	}{
		{
			name:      "Bad SQL",
			sql:       "this is not sql;",
			expectErr: true,
		},
		{
			name:      "Bad SQL, no semicolon",
			sql:       "this is not sql, no semicolon",
			expectErr: true,
		},
		{
			name: "select 1",
			sql:  "select 1",
		},
		{
			name: "select 1;",
			sql:  "select 1",
		},
		{
			name: "multiselect",
			sql:  "select 1; select 2",
		},
		{
			name:      "comments",
			sql:       "select 1; select 2; \n--this is a comment\nselect 3",
			jsonMulti: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// No parallel, to many execs

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			osq, err := NewOsqueryProcess(
				osquerydPath,
				WithStdout(&stdout),
				WithStderr(&stderr),
			)
			require.NoError(t, err)

			if tt.expectErr {
				require.Error(t, osq.RunSql(context.TODO(), []byte(tt.sql)))
				require.Contains(t, stderr.String(), "Error")
				return
			}

			require.NoError(t, osq.RunSql(context.TODO(), []byte(tt.sql)))

			if tt.jsonMulti {
				// This is discouraged, and hard to test. So let's bail for now
				return
			}

			var result []map[string]string
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))

			fmt.Println(stderr.String())
			spew.Dump(result)

		})
	}

}
