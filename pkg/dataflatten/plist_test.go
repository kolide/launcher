package dataflatten

import (
	"testing"
)

// TestPlist is testing a very simple plist case. Most of the more complex testing is in the spec files.
func TestPlist(t *testing.T) {
	t.Parallel()

	var tests = []flattenTestCase{
		{
			in: `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><array><string>a</string><string>b</string></array></plist>`,
			out: []Row{
				Row{Path: []string{"0"}, Value: "a"},
				Row{Path: []string{"1"}, Value: "b"},
			},
		},
		{
			in:  `<?xml version="1.0" encoding="UTF-8"?>`,
			err: true,
		},
	}

	for _, tt := range tests {
		actual, err := Plist([]byte(tt.in))
		testFlattenCase(t, tt, actual, err)
	}
}
