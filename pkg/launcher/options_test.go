package launcher

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigFilePath(t *testing.T) {
	t.Parallel()

	var fallbackConfigFile string
	switch runtime.GOOS {
	case "darwin", "linux":
		fallbackConfigFile = "/etc/kolide-k2/launcher.flags"
	case "windows":
		fallbackConfigFile = `C:\Program Files\Kolide\Launcher-kolide-k2\conf\launcher.flags`
	}

	testCases := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name: "single hyphen",
			args: []string{
				"-some", "arg",
				"-config", "/single/hyphen/path/to/launcher.flags",
				"-another", "arg",
			},
			expected: "/single/hyphen/path/to/launcher.flags",
		},
		{
			name: "double hyphen",
			args: []string{
				"--config", "/double/hyphen/path/to/launcher.flags",
				"--some", "arg",
			},
			expected: "/double/hyphen/path/to/launcher.flags",
		},
		{
			name: "double hyphen and equals",
			args: []string{
				"--arg1=value",
				"--config=/different/path/to/launcher.flags",
			},
			expected: "/different/path/to/launcher.flags",
		},
		{
			name:     "no config file present",
			args:     []string{},
			expected: fallbackConfigFile,
		},
	}

	for _, tt := range testCases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, ConfigFilePath(tt.args), tt.expected)
		})
	}
}
