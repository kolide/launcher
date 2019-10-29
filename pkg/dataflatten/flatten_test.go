package dataflatten

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

type flattenTestCase struct {
	in      string
	out     []Row
	options []FlattenOpts
	comment string
	err     bool
}

func TestRowParentFunctions(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in     Row
		parent string
		key    string
	}{
		{
			in: Row{},
		},

		{
			in: Row{Path: []string{}},
		},
		{
			in:     Row{Path: []string{"a"}},
			parent: "",
			key:    "a",
		},
		{
			in:     Row{Path: []string{"a", "b"}},
			parent: "a",
			key:    "b",
		},
		{
			in:     Row{Path: []string{"a", "b", "c"}},
			parent: "a/b",
			key:    "c",
		},
	}

	for _, tt := range tests {
		parent, key := tt.in.ParentKey("/")
		require.Equal(t, tt.parent, parent)
		require.Equal(t, tt.key, key)
	}
}

func TestExtractKeyNameFromMap(t *testing.T) {
	t.Parallel()

	record := map[string]interface{}{
		"id":   123,
		"uuid": "abc123",
		"name": "alice",
		"favs": []int{3, 4},
	}

	var tests = []struct {
		out     string
		keyName string
	}{
		{
			out: "",
		},
		{
			out:     "",
			keyName: "notHere",
		},
		{
			out:     "123",
			keyName: "id",
		},

		{
			out:     "abc123",
			keyName: "uuid",
		},
	}

	for _, tt := range tests {
		actual := extractKeyNameFromMap(record, tt.keyName, false)
		require.Equal(t, tt.out, actual, `keyName "%s"`, tt.keyName)
	}
}

func TestIsArrayOfMapsWithKeyName(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in      []interface{}
		keyName string
		out     bool
	}{
		{
			in:  nil,
			out: false,
		},
		{
			in:  []interface{}{},
			out: false,
		},
		{
			in:  []interface{}{"a", "b", "c"},
			out: false,
		},
		{
			in: []interface{}{
				map[string]interface{}{"id": 1, "uuid": "abc123", "name": "Alice"},
				map[string]interface{}{"id": 2, "uuid": "def456", "name": "Bob"},
				map[string]interface{}{"id": "3", "uuid": "ghi789", "name": "Charlie"},
			},
			out: false,
		},
		{
			in: []interface{}{
				map[string]interface{}{"id": 4, "uuid": "abc123", "name": "Alice"},
				map[string]interface{}{"id": 5, "uuid": "def456", "name": "Bob"},
				map[string]interface{}{"id": "6", "uuid": "ghi789", "name": "Charlie"},
			},
			keyName: "id",
			out:     true,
		},
		{
			in: []interface{}{
				map[string]interface{}{"id": 7, "uuid": "abc123", "name": "Alice"},
				map[string]interface{}{"id": 8, "uuid": "def456", "name": "Bob"},
				map[string]interface{}{"id": "9", "uuid": "ghi789", "name": "Charlie"},
			},
			keyName: "uuid",
			out:     true,
		},
		{
			in: []interface{}{
				map[string]interface{}{"id": 7, "uuid": "abc123", "name": "Alice"},
				map[string]interface{}{"uuid": "def456", "name": "Bob"},
			},
			keyName: "id",
			out:     false,
		},
	}

	for _, tt := range tests {
		fl := &Flattener{arrayKeyName: tt.keyName}
		actual := fl.isArrayOfMapsWithKeyName(tt.in)
		require.Equal(t, tt.out, actual, tt.in)
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
				Row{Path: []string{"data", "a", "v"}, Value: "1"},
				Row{Path: []string{"data", "b", "v"}, Value: "2"},
				Row{Path: []string{"data", "c", "v"}, Value: "3"},
			},
			options: []FlattenOpts{ArrayKeyName("id")},
			comment: "nested array as map",
		},
	}

	for _, tt := range tests {
		actual, err := Json([]byte(tt.in), tt.options...)
		testFlattenCase(t, tt, actual, err)
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
		actual, err := Json([]byte(tt.in), tt.options...)
		testFlattenCase(t, tt, actual, err)
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
