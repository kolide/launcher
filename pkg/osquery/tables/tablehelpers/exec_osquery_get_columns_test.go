package tablehelpers

import (
	"testing"

	"github.com/kolide/osquery-go/plugin/table"
	"github.com/stretchr/testify/require"
)

func TestParseSqlCreate(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in       string
		expected []table.ColumnDefinition
		err      bool
	}{
		{
			in: "CREATE VIRTUAL TABLE screenlock USING screenlock(`enabled` INTEGER, `grace_period` INTEGER)",
			expected: []table.ColumnDefinition{
				table.ColumnDefinition{Name: "enabled", Type: "INTEGER"},
				table.ColumnDefinition{Name: "grace_period", Type: "INTEGER"},
			},
		},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			actual, err := parseSqlCreate(tt.in)
			if tt.err {
				require.Error(t, err)
				require.Nil(t, actual)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expected, actual)
		})
	}
}
