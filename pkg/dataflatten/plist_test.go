package dataflatten

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPlist is testing a very simple plist case. Most of the more complex testing is in the spec files.
func TestPlist(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in  string
		out []Row
		err bool
	}{
		{
			in: `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><array><string>a</string><string>b</string></array></plist>`,
			out: []Row{
				Row{Path: []string{"0"}, Value: "a"},
				Row{Path: []string{"1"}, Value: "b"},
			},
		},
	}

	for _, tt := range tests {
		actual, err := Plist([]byte(tt.in))
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
