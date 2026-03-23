package dataflatten

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJsoncFile(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName string
		fileName     string
		expectedRows []Row
	}{
		{
			testCaseName: "json not jsonc",
			fileName:     filepath.Join("testdata", "nested.json"),
			expectedRows: []Row{
				{
					Path:  []string{"0", "addons", "0", "name"},
					Value: "Nested Strings",
				},
				{
					Path:  []string{"0", "addons", "0", "nest1", "string3"},
					Value: "string3",
				},
				{
					Path:  []string{"0", "addons", "0", "nest1", "string4"},
					Value: "string4",
				},
				{
					Path:  []string{"0", "addons", "0", "nest1", "string5"},
					Value: "string5",
				},
				{
					Path:  []string{"0", "addons", "0", "nest1", "string6"},
					Value: "string6",
				},
			},
		},
		{
			testCaseName: "simple",
			fileName:     filepath.Join("testdata", "simple.jsonc"),
			expectedRows: []Row{
				{
					Path:  []string{"0", "test3"},
					Value: "true",
				},
				{
					Path:  []string{"0", "test4"},
					Value: "this value has a / in it, but that's okay!",
				},
				{
					Path:  []string{"0", "test5"},
					Value: "this value has a // in it, but that is also fine",
				},
				{
					Path:  []string{"0", "test6"},
					Value: "this value has a /* in it, but we're ignoring that",
				},
				{
					Path:  []string{"0", "test7"},
					Value: "7",
				},
				{
					Path:  []string{"0", "test8", "0", "id"},
					Value: "1",
				},
				{
					Path:  []string{"0", "test8", "1", "id"},
					Value: "2",
				},
				{
					Path:  []string{"0", "test9"},
					Value: "this looks like a trailing comma,] but it is not ,}",
				},
			},
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			rows, err := JsoncFile(tt.fileName)
			require.NoError(t, err)
			require.Equal(t, len(tt.expectedRows), len(rows))

			// Row ordering is not guaranteed, so confirm that all rows in tt.expectedRows are in `rows`
			// (i.e. no data missing), and then that all entries in `rows` are in tt.expectedRows
			// (i.e. no unexpected data)
			for _, expectedRow := range tt.expectedRows {
				require.Contains(t, rows, expectedRow, "missing expected row")
			}
			for _, row := range rows {
				require.Contains(t, tt.expectedRows, row, "received unexpected row")
			}
		})
	}
}

func TestJsonc(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName string
		fileName     string
		expectedRows []Row
	}{
		{
			testCaseName: "json not jsonc",
			fileName:     filepath.Join("testdata", "nested.json"),
			expectedRows: []Row{
				{
					Path:  []string{"0", "addons", "0", "name"},
					Value: "Nested Strings",
				},
				{
					Path:  []string{"0", "addons", "0", "nest1", "string3"},
					Value: "string3",
				},
				{
					Path:  []string{"0", "addons", "0", "nest1", "string4"},
					Value: "string4",
				},
				{
					Path:  []string{"0", "addons", "0", "nest1", "string5"},
					Value: "string5",
				},
				{
					Path:  []string{"0", "addons", "0", "nest1", "string6"},
					Value: "string6",
				},
			},
		},
		{
			testCaseName: "simple",
			fileName:     filepath.Join("testdata", "simple.jsonc"),
			expectedRows: []Row{
				{
					Path:  []string{"0", "test3"},
					Value: "true",
				},
				{
					Path:  []string{"0", "test4"},
					Value: "this value has a / in it, but that's okay!",
				},
				{
					Path:  []string{"0", "test5"},
					Value: "this value has a // in it, but that is also fine",
				},
				{
					Path:  []string{"0", "test6"},
					Value: "this value has a /* in it, but we're ignoring that",
				},
				{
					Path:  []string{"0", "test7"},
					Value: "7",
				},
				{
					Path:  []string{"0", "test8", "0", "id"},
					Value: "1",
				},
				{
					Path:  []string{"0", "test8", "1", "id"},
					Value: "2",
				},
				{
					Path:  []string{"0", "test9"},
					Value: "this looks like a trailing comma,] but it is not ,}",
				},
			},
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			rawdata, err := os.ReadFile(tt.fileName)
			require.NoError(t, err)

			rows, err := Jsonc(rawdata)
			require.NoError(t, err)
			require.Equal(t, len(tt.expectedRows), len(rows))

			// Row ordering is not guaranteed, so confirm that all rows in tt.expectedRows are in `rows`
			// (i.e. no data missing), and then that all entries in `rows` are in tt.expectedRows
			// (i.e. no unexpected data)
			for _, expectedRow := range tt.expectedRows {
				require.Contains(t, rows, expectedRow, "missing expected row")
			}
			for _, row := range rows {
				require.Contains(t, tt.expectedRows, row, "received unexpected row")
			}
		})
	}
}
