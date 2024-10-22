package packaging

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeHostname(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in  string
		out string
	}{
		{
			in:  "hostname",
			out: "hostname",
		},
		{
			in:  "hello:colon",
			out: "hello-colon",
		},
	}

	for _, tt := range tests {
		require.Equal(t, tt.out, sanitizeHostname(tt.in))
	}
}

func Test_osqueryVersionFromVersionOutput(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName    string
		versionOutput   string
		expectedVersion string
	}{
		{
			testCaseName:    "windows",
			versionOutput:   "osqueryd.exe version 5.14.1",
			expectedVersion: "5.14.1",
		},
		{
			testCaseName:    "non-windows",
			versionOutput:   "osqueryd version 5.13.1",
			expectedVersion: "5.13.1",
		},
		{
			testCaseName: "extra spaces",
			versionOutput: `
osqueryd version 5.13.1
`,
			expectedVersion: "5.13.1",
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expectedVersion, osqueryVersionFromVersionOutput(tt.versionOutput))
		})
	}
}
