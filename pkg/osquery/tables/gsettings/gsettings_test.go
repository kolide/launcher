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

func TestGsettingsValues(t *testing.T) {
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

// func TestGsettingsMetadata(t *testing.T) {
// 	t.Parallel()

// 	var tests = []struct {
// 		filename string
// 		expected []map[string]string
// 	}{
// 		{
// 			filename: "simple.txt",
// 			expected: []map[string]string{
// 				{
// 					"fullkey": "org.gnome.rhythmbox.plugins.webremote/access-key",
// 					"parent":  "org.gnome.rhythmbox.plugins.webremote",
// 					"key":     "access-key",
// 					"value":   "''",
// 					"schema":  "org.gnome.rhythmbox.plugins.webremote",
// 				},
// 				{

// 					"fullkey": "org.gnome.rhythmbox.plugins.webremote/foo-bar",
// 					"parent":  "org.gnome.rhythmbox.plugins.webremote",
// 					"key":     "foo-bar",
// 					"value":   "2",
// 					"schema":  "org.gnome.rhythmbox.plugins.webremote",
// 				},
// 			},
// 		},
// 	}

// 	for _, tt := range tests {
// 		table := GsettingsValues{
// 			logger: log.NewNopLogger(),
// 			getBytes: func(ctx context.Context, buf *bytes.Buffer) error {
// 				f, err := os.Open(filepath.Join("testdata", tt.filename))
// 				require.NoError(t, err, "opening file %s", tt.filename)
// 				_, err = buf.ReadFrom(f)
// 				require.NoError(t, err, "read file %s", tt.filename)

// 				return nil
// 			},
// 		}
// 		t.Run(tt.filename, func(t *testing.T) {
// 			ctx := context.TODO()
// 			qCon := tablehelpers.MockQueryContext(map[string][]string{})

// 			results, err := table.generate(ctx, qCon)
// 			require.NoError(t, err, "generating results from %s", tt.filename)
// 			require.ElementsMatch(t, tt.expected, results)
// 		})
// 	}
// }

func TestListKeys(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		filename string
		expected []string
	}{
		{
			filename: "listkeys.txt",
			expected: []string{
				"nmines",
				"window-width",
				"ysize",
				"use-question-marks",
				"use-autoflag",
				"use-animations",
				"mode",
				"xsize",
				"theme",
				"window-height",
				"window-is-maximized",
			},
		},
	}

	for _, tt := range tests {
		table := GsettingsMetadata{
			logger: log.NewNopLogger(),
			cmdRunner: func(ctx context.Context, args []string, buf *bytes.Buffer) error {
				f, err := os.Open(filepath.Join("testdata", tt.filename))
				require.NoError(t, err, "opening file %s", tt.filename)
				_, err = buf.ReadFrom(f)
				require.NoError(t, err, "read file %s", tt.filename)

				return nil
			},
		}
		t.Run(tt.filename, func(t *testing.T) {
			ctx := context.TODO()

			results, err := table.listKeys(ctx, "org.gnome.Mines")
			require.NoError(t, err, "generating results from %s", tt.filename)
			require.ElementsMatch(t, tt.expected, results)
		})
	}
}

func TestGetType(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		input    string
		expected string
	}{
		{
			input:    "type i",
			expected: "int32",
		},
		{
			input:    "range i 4 100",
			expected: "int32 (4 to 100)",
		},
		{
			input: `enum
'artists-albums'
'genres-artists'
'genres-artists-albums'
`,
			expected: "enum: [ 'artists-albums','genres-artists','genres-artists-albums' ]",
		},
		{
			input:    "type as",
			expected: "array of string",
		},
	}

	for _, tt := range tests {
		table := GsettingsMetadata{
			logger: log.NewNopLogger(),
			cmdRunner: func(ctx context.Context, args []string, buf *bytes.Buffer) error {
				_, err := buf.WriteString(tt.input)
				require.NoError(t, err)

				return nil
			},
		}
		t.Run(tt.expected, func(t *testing.T) {
			ctx := context.TODO()

			result, err := table.getType(ctx, "key", "schema")
			require.NoError(t, err, "getting type", tt.expected)
			require.Equal(t, tt.expected, result)
		})
	}
}
