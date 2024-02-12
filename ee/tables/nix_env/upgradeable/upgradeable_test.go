//go:build linux
// +build linux

package nix_env_upgradeable

import (
	"context"
	"os/exec"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueries(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		testfile string
		uid      []string
		len      int
	}{
		{
			testfile: "test_data/empty.output",
			uid:      []string{"1000"},
			len:      1,
		},
		{
			testfile: "test_data/error.output",
			uid:      []string{"1000"},
			len:      0,
		},
		{
			testfile: "test_data/example.output",
			uid:      []string{"1000"},
			len:      18,
		},
	}

	for _, tt := range tests {
		tt := tt
		testTable := &Table{
			logger: log.NewNopLogger(),
			execCC: execFaker(tt.testfile),
		}

		t.Run(tt.testfile, func(t *testing.T) {
			t.Parallel()

			mockQC := tablehelpers.MockQueryContext(map[string][]string{
				"uid": tt.uid,
			})

			rows, err := testTable.generate(context.TODO(), mockQC)

			require.NoError(t, err)
			assert.Equal(t, tt.len, len(rows))
		})
	}

}

func execFaker(filename string) func(context.Context, ...string) (*exec.Cmd, error) {
	return func(ctx context.Context, _ ...string) (*exec.Cmd, error) {
		return exec.CommandContext(ctx, "/bin/cat", filename), nil //nolint:forbidigo // Fine to use exec.CommandContext in test
	}
}
