//go:build darwin

package appicons

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"testing"

	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/require"
)

func Test_generateAppIcons(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			results, err := generateAppIcons(t.Context(), tt.queryContext)
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

			// code blow is a good for sanity check if you want to ensure the files are being decoded properly
			// will save images to ./test_images and you can look and see how pretty they are

			/*

				// Save the decoded PNG under ./test_images using the app name from the query constraint
				err = os.MkdirAll("./test_images", 0o755)
				require.NoError(t, err)

				// derive a file name from the provided path constraint if present
				appPath := ""
				if cl, ok := tt.queryContext.Constraints["path"]; ok && len(cl.Constraints) > 0 {
					appPath = cl.Constraints[0].Expression
				}
				if appPath == "" {
					appPath = "icon"
				}
				name := strings.TrimSuffix(filepath.Base(appPath), ".app")
				outPath := filepath.Join("./test_images", name+".png")

				err = os.WriteFile(outPath, iconBytes, 0o644)
				require.NoError(t, err)
			*/
		})
	}
}
