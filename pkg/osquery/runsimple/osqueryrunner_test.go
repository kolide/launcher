//nolint:paralleltest
package runsimple

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/pkg/packaging"
	"github.com/stretchr/testify/require"
)

var testOsqueryBinary string

// TestMain overrides the default test main function. This allows us to share setup/teardown.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "osquery-runsimple")
	if err != nil {
		fmt.Println("Failed to make temp dir for test binaries")
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit in tests
	}

	if err := downloadOsqueryInBinDir(dir); err != nil {
		fmt.Printf("Failed to download osquery: %v\n", err)
		os.RemoveAll(dir) // explicit removal as defer will not run when os.Exit is called
		os.Exit(1)        //nolint:forbidigo // Fine to use os.Exit in tests
	}

	testOsqueryBinary = filepath.Join(dir, "osqueryd")
	if runtime.GOOS == "windows" {
		testOsqueryBinary += ".exe"
	}

	// Run the tests!
	retCode := m.Run()

	os.RemoveAll(dir) // explicit removal as defer will not run when os.Exit is called
	os.Exit(retCode)  //nolint:forbidigo // Fine to use os.Exit in tests
}

func Test_OsqueryRunSqlNoIO(t *testing.T) {
	osq, err := NewOsqueryProcess(testOsqueryBinary)
	require.NoError(t, err)

	require.NoError(t, osq.RunSql(context.TODO(), []byte("select 1")))
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
				require.Error(t, osq.RunSql(context.TODO(), []byte(tt.sql)))
				require.Contains(t, stderr.String(), "Error")
				return
			}

			require.NoError(t, osq.RunSql(context.TODO(), []byte(tt.sql)))

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

// downloadOsqueryInBinDir downloads osqueryd. This allows the test
// suite to run on hosts lacking osqueryd.
func downloadOsqueryInBinDir(binDirectory string) error {
	target := packaging.Target{}
	if err := target.PlatformFromString(runtime.GOOS); err != nil {
		return fmt.Errorf("Error parsing platform: %s: %w", runtime.GOOS, err)
	}
	target.Arch = packaging.ArchFlavor(runtime.GOARCH)
	if runtime.GOOS == "darwin" {
		target.Arch = packaging.Universal
	}

	outputFile := filepath.Join(binDirectory, target.PlatformBinaryName("osqueryd"))
	cacheDir := binDirectory

	path, err := packaging.FetchBinary(context.TODO(), cacheDir, "osqueryd", target.PlatformBinaryName("osqueryd"), "stable", target)
	if err != nil {
		return fmt.Errorf("An error occurred fetching the osqueryd binary: %w", err)
	}

	if err := fsutil.CopyFile(path, outputFile); err != nil {
		return fmt.Errorf("Couldn't copy file to %s: %w", outputFile, err)
	}

	return nil
}
