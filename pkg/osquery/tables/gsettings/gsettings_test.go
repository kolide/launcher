package gsettings

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/stretchr/testify/require"
)

func TestOutputParsing(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		filename string
		expected []map[string]string
	}{
		{
			filename: "blank.txt",
			expected: []map[string]string{},
		},
		{
			filename: "simple.txt",
			expected: []map[string]string{
				{
					"fullkey": "org.gnome.rhythmbox.plugins.webremote/access-key",
					"parent":  "org.gnome.rhythmbox.plugins.webremote",
					"key":     "access-key",
					"value":   "''",
					"schema":  "org.gnome.rhythmbox.plugins.webremote",
				},
				{

					"fullkey": "org.gnome.rhythmbox.plugins.webremote/foo-bar",
					"parent":  "org.gnome.rhythmbox.plugins.webremote",
					"key":     "foo-bar",
					"value":   "2",
					"schema":  "org.gnome.rhythmbox.plugins.webremote",
				},
			},
		},
	}

	for _, tt := range tests {
		table := GsettingsValues{
			logger: log.NewNopLogger(),
			getBytes: func(ctx context.Context, buf *bytes.Buffer) error {
				f, err := os.Open(filepath.Join("testdata", tt.filename))
				require.NoError(t, err, "opening file %s", tt.filename)
				_, err = buf.ReadFrom(f)
				require.NoError(t, err, "read file %s", tt.filename)

				return nil
			},
		}
		t.Run(tt.filename, func(t *testing.T) {
			ctx := context.TODO()
			qCon := tablehelpers.MockQueryContext(map[string][]string{})

			results, err := table.generate(ctx, qCon)
			require.NoError(t, err, "generating results from %s", tt.filename)
			require.ElementsMatch(t, tt.expected, results)
		})
	}
}
