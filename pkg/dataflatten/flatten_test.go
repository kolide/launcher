package dataflatten

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type flattenTestCase struct {
	in      string
	out     []Row
	options []FlattenOpts
	comment string
	err     bool
}

func TestFlatten_Complex2(t *testing.T) {
	t.Parallel()

	dataRaw, err := ioutil.ReadFile(filepath.Join("testdata", "complex2.json"))
	require.NoError(t, err, "reading file")
	var dataIn interface{}
	require.NoError(t, json.Unmarshal(dataRaw, &dataIn), "unmarshalling json")

	var tests = []flattenTestCase{
		{
			out: []Row{
				Row{Path: []string{"addons", "0", "bool1"}, Value: "true"},
				Row{Path: []string{"addons", "0", "nest2", "0", "string2"}, Value: "foo"},
				Row{Path: []string{"addons", "0", "nest3", "string6"}, Value: "null"},
				Row{Path: []string{"addons", "0", "nest3", "string7"}, Value: "A Very Long Sentence"},
				Row{Path: []string{"addons", "0", "nest3", "string8"}, Value: "short"},
				Row{Path: []string{"addons", "0", "string1"}, Value: "hello"},
			},
		},
		{
			out: []Row{
				Row{Path: []string{"addons", "0", "bool1"}, Value: "true"},
				Row{Path: []string{"addons", "0", "nest2", "0", "null3"}, Value: ""},
				Row{Path: []string{"addons", "0", "nest2", "0", "null4"}, Value: ""},
				Row{Path: []string{"addons", "0", "nest2", "0", "string2"}, Value: "foo"},
				Row{Path: []string{"addons", "0", "nest3", "string3"}, Value: ""},
				Row{Path: []string{"addons", "0", "nest3", "string4"}, Value: ""},
				Row{Path: []string{"addons", "0", "nest3", "string5"}, Value: ""},
				Row{Path: []string{"addons", "0", "nest3", "string6"}, Value: "null"},
				Row{Path: []string{"addons", "0", "nest3", "string7"}, Value: "A Very Long Sentence"},
				Row{Path: []string{"addons", "0", "nest3", "string8"}, Value: "short"},
				Row{Path: []string{"addons", "0", "null1"}, Value: ""},
				Row{Path: []string{"addons", "0", "null2"}, Value: ""},
				Row{Path: []string{"addons", "0", "string1"}, Value: "hello"},
			},
			options: []FlattenOpts{IncludeNulls()},
			comment: "includes null",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.comment, func(t *testing.T) {
			t.Parallel()

			actual, err := Flatten(dataIn, tt.options...)
			testFlattenCase(t, tt, actual, err)
		})
	}

}

func TestFlatten_NestingBug(t *testing.T) {
	t.Parallel()

	dataRaw, err := ioutil.ReadFile(filepath.Join("testdata", "nested.json"))
	require.NoError(t, err, "reading file")
	var dataIn interface{}
	require.NoError(t, json.Unmarshal(dataRaw, &dataIn), "unmarshalling json")

	var tests = []flattenTestCase{
		{
			out: []Row{
				Row{Path: []string{"addons", "0", "name"}, Value: "Nested Strings"},
				Row{Path: []string{"addons", "0", "nest1", "string3"}, Value: "string3"},
				Row{Path: []string{"addons", "0", "nest1", "string4"}, Value: "string4"},
				Row{Path: []string{"addons", "0", "nest1", "string5"}, Value: "string5"},
				Row{Path: []string{"addons", "0", "nest1", "string6"}, Value: "string6"},
			},
		},
		{
			out: []Row{
				Row{Path: []string{"addons", "0", "name"}, Value: "Nested Strings"},
				Row{Path: []string{"addons", "0", "nest1", "string1"}, Value: ""},
				Row{Path: []string{"addons", "0", "nest1", "string2"}, Value: ""},
				Row{Path: []string{"addons", "0", "nest1", "string3"}, Value: "string3"},
				Row{Path: []string{"addons", "0", "nest1", "string4"}, Value: "string4"},
				Row{Path: []string{"addons", "0", "nest1", "string5"}, Value: "string5"},
				Row{Path: []string{"addons", "0", "nest1", "string6"}, Value: "string6"},
			},
			options: []FlattenOpts{IncludeNulls()},
			comment: "includes null",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.comment, func(t *testing.T) {
			t.Parallel()

			actual, err := Flatten(dataIn, tt.options...)
			testFlattenCase(t, tt, actual, err)
		})
	}

}

func TestFlatten_Complex(t *testing.T) {
	t.Parallel()

	// Do the unmarshaling here, so we don't keep doing it again and again
	dataRaw, err := ioutil.ReadFile(filepath.Join("testdata", "animals.json"))
	require.NoError(t, err, "reading file")
	var dataIn interface{}
	require.NoError(t, json.Unmarshal(dataRaw, &dataIn), "unmarshalling json")

	// We do a bunch of tests to select this user. So we'll pull
	// this out here and make the testcases more DRY
	testdataUser0 := []Row{
		Row{Path: []string{"users", "0", "favorites", "0"}, Value: "ants"},
		Row{Path: []string{"users", "0", "id"}, Value: "1"},
		Row{Path: []string{"users", "0", "name"}, Value: "Alex Aardvark"},
		Row{Path: []string{"users", "0", "uuid"}, Value: "abc123"},
	}

	var tests = []flattenTestCase{
		{
			out: []Row{
				Row{Path: []string{"metadata", "testing"}, Value: "true"},
				Row{Path: []string{"metadata", "version"}, Value: "1.0.1"},
				Row{Path: []string{"system"}, Value: "users demo"},
				Row{Path: []string{"users", "0", "favorites", "0"}, Value: "ants"},
				Row{Path: []string{"users", "0", "id"}, Value: "1"},
				Row{Path: []string{"users", "0", "name"}, Value: "Alex Aardvark"},
				Row{Path: []string{"users", "0", "uuid"}, Value: "abc123"},
				Row{Path: []string{"users", "1", "favorites", "0"}, Value: "mice"},
				Row{Path: []string{"users", "1", "favorites", "1"}, Value: "birds"},
				Row{Path: []string{"users", "1", "id"}, Value: "2"},
				Row{Path: []string{"users", "1", "name"}, Value: "Bailey Bobcat"},
				Row{Path: []string{"users", "1", "uuid"}, Value: "def456"},
				Row{Path: []string{"users", "2", "favorites", "0"}, Value: "seeds"},
				Row{Path: []string{"users", "2", "id"}, Value: "3"},
				Row{Path: []string{"users", "2", "name"}, Value: "Cam Chipmunk"},
				Row{Path: []string{"users", "2", "uuid"}, Value: "ghi789"},
			},
			comment: "all together",
		},
		{
			options: []FlattenOpts{WithQuery([]string{"metadata"})},
			out: []Row{
				Row{Path: []string{"metadata", "testing"}, Value: "true"},
				Row{Path: []string{"metadata", "version"}, Value: "1.0.1"},
			},
		},
		{
			comment: "array by #",
			options: []FlattenOpts{WithQuery([]string{"users", "0"})},
			out:     testdataUser0,
		},
		{
			comment: "array by id value",
			options: []FlattenOpts{WithQuery([]string{"users", "id=>1"})},
			out:     testdataUser0,
		},
		{
			comment: "array by uuid",
			options: []FlattenOpts{WithQuery([]string{"users", "uuid=>abc123"})},
			out:     testdataUser0,
		},
		{
			comment: "array by name with suffix wildcard",
			options: []FlattenOpts{WithQuery([]string{"users", "name=>Al*"})},
			out:     testdataUser0,
		},
		{
			comment: "array by name with prefix wildcard",
			options: []FlattenOpts{WithQuery([]string{"users", "name=>*Aardvark"})},
			out:     testdataUser0,
		},

		{
			comment: "array by name with suffix and prefix",
			options: []FlattenOpts{WithQuery([]string{"users", "name=>*Aardv*"})},
			out:     testdataUser0,
		},
		{
			comment: "who likes ants, array re-written",
			options: []FlattenOpts{WithQuery([]string{"users", "#name", "favorites", "ants"})},
			out: []Row{
				Row{Path: []string{"users", "Alex Aardvark", "favorites", "0"}, Value: "ants"},
			},
		},
		{
			comment: "rewritten and filtered",
			options: []FlattenOpts{WithQuery([]string{"users", "#name=>Al*", "id"})},
			out: []Row{
				Row{Path: []string{"users", "Alex Aardvark", "id"}, Value: "1"},
			},
		},
		{
			comment: "bad key name",
			options: []FlattenOpts{WithQuery([]string{"users", "#nokey"})},
			out:     []Row{},
		},
		{
			comment: "rewrite array to map",
			options: []FlattenOpts{WithQuery([]string{"users", "#name", "id"})},
			out: []Row{
				Row{Path: []string{"users", "Alex Aardvark", "id"}, Value: "1"},
				Row{Path: []string{"users", "Bailey Bobcat", "id"}, Value: "2"},
				Row{Path: []string{"users", "Cam Chipmunk", "id"}, Value: "3"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.comment, func(t *testing.T) {
			t.Parallel()

			actual, err := Flatten(dataIn, tt.options...)
			testFlattenCase(t, tt, actual, err)
		})
	}
}

func TestFlatten_ArrayMaps(t *testing.T) {
	t.Parallel()

	var tests = []flattenTestCase{
		{
			in: `{"data": [{"v":1,"id":"a"},{"v":2,"id":"b"},{"v":3,"id":"c"}]}`,
			out: []Row{
				Row{Path: []string{"data", "0", "id"}, Value: "a"},
				Row{Path: []string{"data", "0", "v"}, Value: "1"},

				Row{Path: []string{"data", "1", "id"}, Value: "b"},
				Row{Path: []string{"data", "1", "v"}, Value: "2"},

				Row{Path: []string{"data", "2", "id"}, Value: "c"},
				Row{Path: []string{"data", "2", "v"}, Value: "3"},
			},
			comment: "nested array as array",
		},
		{
			in: `{"data": [{"v":1,"id":"a"},{"v":2,"id":"b"},{"v":3,"id":"c"}]}`,
			out: []Row{
				Row{Path: []string{"data", "a", "id"}, Value: "a"},
				Row{Path: []string{"data", "a", "v"}, Value: "1"},

				Row{Path: []string{"data", "b", "id"}, Value: "b"},
				Row{Path: []string{"data", "b", "v"}, Value: "2"},

				Row{Path: []string{"data", "c", "id"}, Value: "c"},
				Row{Path: []string{"data", "c", "v"}, Value: "3"},
			},
			options: []FlattenOpts{WithQuery([]string{"data", "#id"})},
			comment: "nested array as map",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.comment, func(t *testing.T) {
			t.Parallel()

			actual, err := Json([]byte(tt.in), tt.options...)
			testFlattenCase(t, tt, actual, err)
		})
	}

}

func TestFlatten(t *testing.T) {
	t.Parallel()

	var tests = []flattenTestCase{
		{
			in:  "a",
			err: true,
		},
		{
			in: `["a", null]`,
			out: []Row{
				Row{Path: []string{"0"}, Value: "a"},
			},
			comment: "skip null",
		},

		{
			in: `["a", "b", null]`,
			out: []Row{
				Row{Path: []string{"0"}, Value: "a"},
				Row{Path: []string{"1"}, Value: "b"},
				Row{Path: []string{"2"}, Value: ""},
			},
			options: []FlattenOpts{IncludeNulls()},
			comment: "includes null",
		},

		{
			in: `["1"]`,
			out: []Row{
				Row{Path: []string{"0"}, Value: "1"},
			},
		},
		{
			in: `["a", true, false, "1", 2, 3.3]`,
			out: []Row{
				Row{Path: []string{"0"}, Value: "a"},
				Row{Path: []string{"1"}, Value: "true"},
				Row{Path: []string{"2"}, Value: "false"},
				Row{Path: []string{"3"}, Value: "1"},
				Row{Path: []string{"4"}, Value: "2"},
				Row{Path: []string{"5"}, Value: "3.3"},
			},
			comment: "mixed types",
		},
		{
			in: `{"a": 1, "b": "2.2", "c": [1,2,3]}`,
			out: []Row{
				Row{Path: []string{"a"}, Value: "1"},
				Row{Path: []string{"b"}, Value: "2.2"},
				Row{Path: []string{"c", "0"}, Value: "1"},
				Row{Path: []string{"c", "1"}, Value: "2"},
				Row{Path: []string{"c", "2"}, Value: "3"},
			},
			comment: "nested types",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.comment, func(t *testing.T) {
			t.Parallel()

			actual, err := Json([]byte(tt.in), tt.options...)
			testFlattenCase(t, tt, actual, err)
		})
	}
}

// testFlattenCase runs tests for a single test case. Normally this
// would be in a for loop, instead it's abstracted here to make it
// simpler to split up a giant array of test cases.
func testFlattenCase(t *testing.T, tt flattenTestCase, actual []Row, actualErr error) {
	if tt.err {
		require.Error(t, actualErr, "test %s %s", tt.in, tt.comment)
		return
	}

	require.NoError(t, actualErr, "test %s %s", tt.in, tt.comment)

	// Despite being an array. data is returned
	// unordered. This greatly complicates our testing. We
	// can either sort it, or use an unordered comparison
	// operator. The `require.ElementsMatch` produces much
	// harder to read diffs, so instead we'll sort things.
	sort.SliceStable(tt.out, func(i, j int) bool { return tt.out[i].StringPath("/") < tt.out[j].StringPath("/") })
	sort.SliceStable(actual, func(i, j int) bool { return actual[i].StringPath("/") < actual[j].StringPath("/") })
	require.EqualValues(t, tt.out, actual, "test %s %s", tt.in, tt.comment)
}

func TestFlattenSliceOfMaps(t *testing.T) {
	t.Parallel()

	type args struct {
		data interface{}
		opts []FlattenOpts
	}
	tests := []struct {
		name      string
		args      args
		want      []Row
		assertion assert.ErrorAssertionFunc
	}{
		{
			name: "single",
			args: args{
				data: []map[string]interface{}{
					{
						"id": "a",
						"v":  1,
					},
				},
				opts: []FlattenOpts{},
			},
			want: []Row{
				{Path: []string{"0", "id"}, Value: "a"},
				{Path: []string{"0", "v"}, Value: "1"},
			},
			assertion: assert.NoError,
		},
		{
			name: "multiple",
			args: args{
				data: []map[string]interface{}{
					{
						"id": "a",
						"v":  1,
					},
					{
						"id": "b",
						"v":  2,
					},
					{
						"id": "c",
						"v":  3,
					},
				},
				opts: []FlattenOpts{},
			},
			want: []Row{
				{Path: []string{"0", "id"}, Value: "a"},
				{Path: []string{"0", "v"}, Value: "1"},
				{Path: []string{"1", "id"}, Value: "b"},
				{Path: []string{"1", "v"}, Value: "2"},
				{Path: []string{"2", "id"}, Value: "c"},
				{Path: []string{"2", "v"}, Value: "3"},
			},
			assertion: assert.NoError,
		},
		{
			name: "error",
			args: args{
				data: []map[string]interface{}{
					{
						"id": []string{"this should cause an error"},
					},
				},
				opts: []FlattenOpts{},
			},
			want:      nil,
			assertion: assert.Error,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Flatten(tt.args.data, tt.args.opts...)
			tt.assertion(t, err)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
