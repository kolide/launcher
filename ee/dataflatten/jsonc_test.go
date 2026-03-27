package dataflatten

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJsonc(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName string
		input        []byte
		expectedRows []Row
	}{
		{
			testCaseName: "regular json",
			input:        []byte(`{"id": 1}`),
			expectedRows: []Row{
				{
					Path:  []string{"0", "id"},
					Value: "1",
				},
			},
		},
		{
			testCaseName: "includes single-line mode comment",
			input: []byte(`
// -*- mode: jsonc -*-
{
	"id": 1
}`),
			expectedRows: []Row{
				{
					Path:  []string{"0", "id"},
					Value: "1",
				},
			},
		},
		{
			testCaseName: "multiple single-line comments",
			input: []byte(`
{
	"id": 1,
	// Here's a single-line comment that spans a couple lines anyway
    //
    // Here's a little more of that comment
	"uuid": "abc"
}`),
			expectedRows: []Row{
				{
					Path:  []string{"0", "id"},
					Value: "1",
				},
				{
					Path:  []string{"0", "uuid"},
					Value: "abc",
				},
			},
		},
		{
			testCaseName: "multi-line comment",
			input: []byte(`
{
	"id": 1,
	/*
        Here's a multi-line comment.
        It has multiple lines.
        That's the whole point.
    */
    "test3": true
}`),
			expectedRows: []Row{
				{
					Path:  []string{"0", "id"},
					Value: "1",
				},
				{
					Path:  []string{"0", "test3"},
					Value: "true",
				},
			},
		},
		{
			testCaseName: "single-line comment containing string",
			input: []byte(`
{
	// Here's a single-line comment: "it's got a string in it!"
    "test3": true
}`),
			expectedRows: []Row{
				{
					Path:  []string{"0", "test3"},
					Value: "true",
				},
			},
		},
		{
			testCaseName: "multi-line comment containing string",
			input: []byte(`
{
	/*
        Here's a multi-line comment.
        "It's also got a string in it!"
    */
    "test3": true
}`),
			expectedRows: []Row{
				{
					Path:  []string{"0", "test3"},
					Value: "true",
				},
			},
		},
		{
			testCaseName: "multi-line comment containing asterisks",
			input: []byte(`
{
	/*
        Here's a multi-line comment that uses asterisks for bullet points.
            * point one
            * point two
            * you get the point
    */
    "enabled": true
}`),
			expectedRows: []Row{
				{
					Path:  []string{"0", "enabled"},
					Value: "true",
				},
			},
		},
		{
			testCaseName: "string containing slash",
			input: []byte(`
{
	"test4": "this value has a / in it, but that's okay!",
    "test3": true
}`),
			expectedRows: []Row{
				{
					Path:  []string{"0", "test4"},
					Value: "this value has a / in it, but that's okay!",
				},
				{
					Path:  []string{"0", "test3"},
					Value: "true",
				},
			},
		},
		{
			testCaseName: "string containing double slash",
			input: []byte(`
{
	"test5": "this value has a // in it, but that is also fine"
}`),
			expectedRows: []Row{
				{
					Path:  []string{"0", "test5"},
					Value: "this value has a // in it, but that is also fine",
				},
			},
		},
		{
			testCaseName: "string containing /*",
			input: []byte(`
{
	"test6": "this value has a /* in it, but we're ignoring that"
}`),
			expectedRows: []Row{
				{
					Path:  []string{"0", "test6"},
					Value: "this value has a /* in it, but we're ignoring that",
				},
			},
		},
		{
			testCaseName: "empty multi-line comment",
			input: []byte(`
{
	"id": "100",
    /**/
}`),
			expectedRows: []Row{
				{
					Path:  []string{"0", "id"},
					Value: "100",
				},
			},
		},
		{
			testCaseName: "comment at end of line",
			input: []byte(`
{
	"age": 99, // this comment starts at the end of a line
}`),
			expectedRows: []Row{
				{
					Path:  []string{"0", "age"},
					Value: "99",
				},
			},
		},
		{
			testCaseName: "multiple trailing commas",
			input: []byte(`
{
	"test8": [
        /*
            Here's a nested comment.
        */
        {
            "id": 1,
        },
        {
            "id": 2
        },
    ],
}`),
			expectedRows: []Row{
				{
					Path:  []string{"0", "test8", "0", "id"},
					Value: "1",
				},
				{
					Path:  []string{"0", "test8", "1", "id"},
					Value: "2",
				},
			},
		},
		{
			testCaseName: "trailing commas inside strings ignored",
			input: []byte(`
{
	"test9": "this looks like a trailing comma,] but it is not ,}",
}`),
			expectedRows: []Row{
				{
					Path:  []string{"0", "test9"},
					Value: "this looks like a trailing comma,] but it is not ,}",
				},
			},
		},
		{
			testCaseName: "literal backslash",
			input: []byte(`
{
	"literal_backslash": "path\\",
}`),
			expectedRows: []Row{
				{
					Path:  []string{"0", "literal_backslash"},
					Value: "path\\",
				},
			},
		},
		{
			testCaseName: "escaping quotes",
			input: []byte(`
{
	"test10": "escaped quotes are \"totally fine, even when // and /* are inside them\\\"",
}`),
			expectedRows: []Row{
				{
					Path:  []string{"0", "test10"},
					Value: "escaped quotes are \"totally fine, even when // and /* are inside them\\\"",
				},
			},
		},
		{
			testCaseName: "trailing comma with comment afterward",
			input: []byte(`
{
	"id": "4", // <- trailing comma with comment afterward
}`),
			expectedRows: []Row{
				{
					Path:  []string{"0", "id"},
					Value: "4",
				},
			},
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			rows, err := Jsonc(tt.input)
			require.NoError(t, err)
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

// TestJsoncFile is a quick test to ensure that file reading works as expected.
// The majority of the logic is tested above in TestJsonc.
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
