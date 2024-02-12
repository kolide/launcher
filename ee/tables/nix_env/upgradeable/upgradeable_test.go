//go:build linux
// +build linux

package nix_env_upgradeable

import (
	"context"
	"os/exec"
	"path"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/stretchr/testify/require"
)

func TestQueries(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name string
		file string
		uid  []string
	}{
		{
			name: "no data",
			file: path.Join("test_data", "empty.output"),
			uid:  []string{"1000"},
		},
		{
			name: "recursion error on profile",
			file: path.Join("test_data", "error.output"),
			uid:  []string{"1001"},
		},
		{
			name: "example query data",
			file: path.Join("test_data", "example.output"),
			uid:  []string{"1002"},
		},
	}

	for _, tt := range tests {
		tt := tt
		testTable := &Table{
			logger: log.NewNopLogger(),
			execCC: execFaker(tt.file),
		}

		testName := tt.file + "/" + tt.name
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			mockQC := tablehelpers.MockQueryContext(map[string][]string{
				"uid": tt.uid,
			})

			_, err := testTable.generate(context.TODO(), mockQC)

			require.NoError(t, err)
		})
	}

}

func execFaker(filename string) func(context.Context, ...string) (*exec.Cmd, error) {
	return func(ctx context.Context, _ ...string) (*exec.Cmd, error) {
		return exec.CommandContext(ctx, "/bin/cat", filename), nil //nolint:forbidigo // Fine to use exec.CommandContext in test
	}
}
