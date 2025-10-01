//go:build darwin

package appicons

import (
	"bytes"
	"context"
	"encoding/base64"
	"image/png"
	"testing"

	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/require"
)

func Test_generateAppIcons(t *testing.T) {
	tests := []struct {
		name         string
		queryContext table.QueryContext
	}{
		{
			name: "happy path",
			queryContext: table.QueryContext{
				Constraints: map[string]table.ConstraintList{
					"path": {Affinity: "TEXT", Constraints: []table.Constraint{{Operator: table.OperatorEquals, Expression: "/Applications/safari.app"}}},
				},
			},
		},
		{
			name: "not a real file", // this just returns a generic file icon on macOS "looks like a sheet of paper with a little fold in the corner"
			queryContext: table.QueryContext{
				Constraints: map[string]table.ConstraintList{
					"path": {Affinity: "TEXT", Constraints: []table.Constraint{{Operator: table.OperatorEquals, Expression: "/not_a_real_file.app"}}},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := generateAppIcons(context.Background(), tt.queryContext)
			require.NoError(t, err)
			require.Len(t, results, 1)
			iconB64 := results[0]["icon"]
			require.NotEmpty(t, iconB64)

			iconBytes, err := base64.StdEncoding.DecodeString(iconB64)
			require.NoError(t, err)

			img, err := png.Decode(bytes.NewReader(iconBytes))
			require.NoError(t, err)
			require.Equal(t, 128, img.Bounds().Dx())
			require.Equal(t, 128, img.Bounds().Dy())

			require.NotEmpty(t, results[0]["hash"])
		})
	}
}
