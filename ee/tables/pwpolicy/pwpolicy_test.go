//go:build darwin

package pwpolicy

import (
	"context"
	"os/exec"
	"path"
	"testing"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueries(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name        string
		file        string
		queryClause []string
		len         int
		err         bool
	}{
		{
			name: "no data, just languages",
			file: path.Join("testdata", "empty.output"),
			len:  41,
		},
		{
			file: path.Join("testdata", "test1.output"),
			len:  148,
		},
		{
			file:        path.Join("testdata", "test1.output"),
			queryClause: []string{"policyCategoryAuthentication"},
			len:         8,
		},
	}

	for _, tt := range tests {
		testTable := &Table{
			slogger: multislogger.NewNopLogger(),
			execCC:  execFaker{filename: tt.file},
		}

		testName := tt.file + "/" + tt.name
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			mockQC := tablehelpers.MockQueryContext(map[string][]string{
				"query": tt.queryClause,
			})

			rows, err := testTable.generate(t.Context(), mockQC)

			if tt.err {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			assert.Equal(t, tt.len, len(rows))
		})
	}

}

type execFaker struct {
	filename string
}

func (execFaker) Name() string { return "execFaker" }

func (ac execFaker) Cmd(ctx context.Context, arg ...string) (*allowedcmd.TracedCmd, error) {
	return &allowedcmd.TracedCmd{
		Ctx: ctx,
		Cmd: exec.CommandContext(ctx, "/bin/cat", ac.filename), //nolint:forbidigo // Fine to use exec.CommandContext in test
	}, nil
}
