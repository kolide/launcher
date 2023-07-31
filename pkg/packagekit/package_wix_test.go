package packagekit

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGenerateMicrosoftProductCode tests that our guid generation is
// stable. These are various guids that we used in production.
func TestGenerateMicrosoftProductCode(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		ident1 string
		identN []string
		out    string
	}{
		{
			ident1: "launcherkolide",
			out:    "0D597685-1969-5D11-B2D6-600939967590",
		},
		{
			ident1: "launcherkolide",
			identN: []string{},
			out:    "0D597685-1969-5D11-B2D6-600939967590",
		},
		{
			ident1: "launcherkolide-app",
			out:    "0FF3EBE1-C157-5C0D-9B74-C15097E024B5",
		},
		{
			ident1: "launcherkolide-app",
			identN: []string{"0.7.0", "386"},
			out:    "F569EA5A-C60A-5952-AE82-14FCF2BF15EC",
		},
		{
			ident1: "launcherkolide-app",
			identN: []string{"0.7.0", "amd64"},
			out:    "8DDC9786-A19D-5BA2-BEB2-0999F959EEC7",
		},
	}

	for _, tt := range tests {
		guid := generateMicrosoftProductCode(tt.ident1, tt.identN...)
		require.Equal(t, len("XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX"), len(guid))
		require.Equal(t, tt.out, guid)
	}
}

func Test_getSigntoolPath(t *testing.T) {
	t.Parallel()

	signtoolPath, err := getSigntoolPath()

	switch runtime.GOOS {
	case "windows":
		// We should expect to find signtool somewhere.
		require.NoError(t, err, "did not expect error finding signtool")
		require.True(t, strings.HasSuffix(signtoolPath, "signtool.exe"))
	case "darwin", "linux":
		// Tests the case where signtool.exe won't be found.
		require.Error(t, err, "expected to error when signtool is not present")
	}
}
