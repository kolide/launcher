package dataflatten

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

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
		parent, key := tt.in.ParentKey()
		require.Equal(t, tt.parent, parent)
		require.Equal(t, tt.key, key)
	}
}

func TestFlatten(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in  string
		out []Row
		err bool
	}{
		{
			in:  "a",
			err: true,
		},
		{
			in: `["a"]`,
			out: []Row{
				Row{Path: []string{"0"}, Value: "a"},
			},
		},
		{
			in: `["1"]`,
			out: []Row{
				Row{Path: []string{"0"}, Value: "1"},
			},
		},
		{
			in: `["a", true, false, 1, 2, 3.3]`,
			out: []Row{
				Row{Path: []string{"0"}, Value: "a"},
				Row{Path: []string{"1"}, Value: "true"},
				Row{Path: []string{"2"}, Value: "false"},
				Row{Path: []string{"3"}, Value: "1"},
				Row{Path: []string{"4"}, Value: "2"},
				Row{Path: []string{"5"}, Value: "3.3"},
			},
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
		},
	}

	for _, tt := range tests {
		actual, err := Json([]byte(tt.in))
		if tt.err {
			require.Error(t, err, tt.in)
			continue
		}

		require.NoError(t, err, tt.in)

		// Despite being an array. data is returned
		// unordered. This greatly complicates our testing. We
		// can either sort it, or use an unordered comparison
		// operator. The `require.ElementsMatch` produces much
		// harder to read diffs, so instead we'll sort things.
		sort.SliceStable(tt.out, func(i, j int) bool { return tt.out[i].StringPath() < tt.out[j].StringPath() })
		sort.SliceStable(actual, func(i, j int) bool { return actual[i].StringPath() < actual[j].StringPath() })
		require.EqualValues(t, tt.out, actual, tt.in)
	}
}
