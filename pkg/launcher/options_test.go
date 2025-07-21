package launcher

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kolide/kit/stringutil"
	"github.com/stretchr/testify/assert"
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

// TestOptionsFromFlags isn't parallel to ensure that we don't pollute the environment
func TestOptionsFromFlags(t *testing.T) { //nolint:paralleltest
	os.Clearenv()

	testArgs, expectedOpts := getArgsAndResponse()

	testFlags := []string{}
	for k, v := range testArgs {
		testFlags = append(testFlags, k)
		if v != "" {
			testFlags = append(testFlags, v)
		}
	}

	opts, err := ParseOptions("", testFlags)
	require.NoError(t, err)
	require.Equal(t, expectedOpts, opts)
}

func TestOptionsFromEnv(t *testing.T) { //nolint:paralleltest

	if runtime.GOOS == "windows" {
		t.Skip("TODO: Windows Testing")
	}

	os.Clearenv()

	testArgs, expectedOpts := getArgsAndResponse()

	for k, val := range testArgs {
		if val == "" {
			val = "true"
		}
		name := fmt.Sprintf("KOLIDE_LAUNCHER_%s", strings.ToUpper(strings.TrimLeft(k, "-")))
		t.Setenv(name, val)
	}
	opts, err := ParseOptions("", []string{})
	require.NoError(t, err)
	require.Equal(t, expectedOpts, opts)
}

func TestOptionsFromFile(t *testing.T) { // nolint:paralleltest
	os.Clearenv()

	testArgs, expectedOpts := getArgsAndResponse()

	flagFile, err := os.CreateTemp(t.TempDir(), "flag-file")
	require.NoError(t, err)
	expectedOpts.ConfigFilePath = flagFile.Name()

	for k, val := range testArgs {
		var err error

		_, err = flagFile.WriteString(strings.TrimLeft(k, "-"))
		require.NoError(t, err)

		if val != "" {
			_, err = flagFile.WriteString(fmt.Sprintf(" %s", val))
			require.NoError(t, err)
		}

		_, err = flagFile.WriteString("\n")
		require.NoError(t, err)
	}

	require.NoError(t, flagFile.Close())

	opts, err := ParseOptions("", []string{"-config", flagFile.Name()})
	require.NoError(t, err)
	require.Equal(t, expectedOpts, opts)
}

func TestAutoupdateDownloadSPlayCanBeDisabledFromFlagsFile(t *testing.T) { //nolint:paralleltest
	os.Clearenv()

	testArgs, expectedOpts := getArgsAndResponse()
	// add in our disable flag to be written to flags file
	testArgs["autoupdate_download_splay"] = "0s"
	// update expectations to require that the resulting options have download splay disabled
	expectedOpts.AutoupdateDownloadSplay = 0 * time.Second

	flagFile, err := os.CreateTemp(t.TempDir(), "flag-file")
	require.NoError(t, err)
	expectedOpts.ConfigFilePath = flagFile.Name()

	for k, val := range testArgs {
		var err error

		_, err = flagFile.WriteString(strings.TrimLeft(k, "-"))
		require.NoError(t, err)

		if val != "" {
			_, err = flagFile.WriteString(fmt.Sprintf(" %s", val))
			require.NoError(t, err)
		}

		_, err = flagFile.WriteString("\n")
		require.NoError(t, err)
	}

	require.NoError(t, flagFile.Close())

	opts, err := ParseOptions("", []string{"-config", flagFile.Name()})
	require.NoError(t, err)
	require.Equal(t, expectedOpts, opts)
}

func TestOptionsSetControlServerHost(t *testing.T) { // nolint:paralleltest
	testCases := []struct {
		testName                   string
		testFlags                  []string
		expectedControlServer      string
		expectedInsecureControlTLS bool
		expectedDisableControlTLS  bool
	}{
		{
			testName: "k2-prod",
			testFlags: []string{
				"--hostname", "k2device.kolide.com",
				"--osqueryd_path", windowsAddExe("/dev/null"),
			},
			expectedControlServer:      "k2control.kolide.com",
			expectedInsecureControlTLS: false,
			expectedDisableControlTLS:  false,
		},
		{
			testName: "k2-preprod",
			testFlags: []string{
				"--hostname", "k2device-preprod.kolide.com",
				"--osqueryd_path", windowsAddExe("/dev/null"),
			},
			expectedControlServer:      "k2control-preprod.kolide.com",
			expectedInsecureControlTLS: false,
			expectedDisableControlTLS:  false,
		},
		{
			testName: "heroku",
			testFlags: []string{
				"--hostname", "test.herokuapp.com",
				"--osqueryd_path", windowsAddExe("/dev/null"),
			},
			expectedControlServer:      "test.herokuapp.com",
			expectedInsecureControlTLS: false,
			expectedDisableControlTLS:  false,
		},
		{
			testName: "localhost with TLS",
			testFlags: []string{
				"--hostname", "localhost:3443",
				"--osqueryd_path", windowsAddExe("/dev/null"),
			},
			expectedControlServer:      "localhost:3443",
			expectedInsecureControlTLS: true,
			expectedDisableControlTLS:  false,
		},
		{
			testName: "localhost without TLS",
			testFlags: []string{
				"--hostname", "localhost:3000",
				"--osqueryd_path", windowsAddExe("/dev/null"),
			},
			expectedControlServer:      "localhost:3000",
			expectedInsecureControlTLS: false,
			expectedDisableControlTLS:  true,
		},
		{
			testName: "unknown host option",
			testFlags: []string{
				"--hostname", "example.com",
				"--osqueryd_path", windowsAddExe("/dev/null"),
			},
			expectedControlServer:      "",
			expectedInsecureControlTLS: false,
			expectedDisableControlTLS:  false,
		},
	}

	for _, tt := range testCases { // nolint:paralleltest
		tt := tt
		os.Clearenv()
		t.Run(tt.testName, func(t *testing.T) {
			opts, err := ParseOptions("", tt.testFlags)
			require.NoError(t, err, "could not parse options")
			require.Equal(t, tt.expectedControlServer, opts.ControlServerURL, "incorrect control server")
			require.Equal(t, tt.expectedInsecureControlTLS, opts.InsecureControlTLS, "incorrect insecure TLS")
			require.Equal(t, tt.expectedDisableControlTLS, opts.DisableControlTLS, "incorrect disable control TLS")
		})
	}
}

func getArgsAndResponse() (map[string]string, *Options) {
	randomHostname := fmt.Sprintf("%s.example.com", stringutil.RandomString(8))
	randomInt := rand.Intn(1024)

	// includes both `-` and `--` for variety.
	args := map[string]string{
		"--hostname":           randomHostname,
		"-autoupdate_interval": "48h",
		"-logging_interval":    fmt.Sprintf("%ds", randomInt),
		"-osqueryd_path":       windowsAddExe("/dev/null"),
	}

	opts := &Options{
		AutoupdateInitialDelay:          1 * time.Hour,
		AutoupdateInterval:              48 * time.Hour,
		Control:                         false,
		ControlServerURL:                "",
		ControlRequestInterval:          60 * time.Second,
		ExportTraces:                    false,
		TraceSamplingRate:               0.0,
		LogIngestServerURL:              "",
		DisableTraceIngestTLS:           false,
		KolideServerURL:                 randomHostname,
		LoggingInterval:                 time.Duration(randomInt) * time.Second,
		MirrorServerURL:                 "https://dl.kolide.co",
		TufServerURL:                    "https://tuf.kolide.com",
		OsquerydPath:                    windowsAddExe("/dev/null"),
		OsqueryHealthcheckStartupDelay:  10 * time.Minute,
		Transport:                       "jsonrpc",
		UpdateChannel:                   "stable",
		DelayStart:                      0 * time.Second,
		WatchdogEnabled:                 false,
		WatchdogDelaySec:                120,
		WatchdogMemoryLimitMB:           600,
		WatchdogUtilizationLimitPercent: 50,
		Identifier:                      DefaultLauncherIdentifier,
		AutoupdateDownloadSplay:         8 * time.Hour,
	}

	return args, opts
}

func TestSanitizeUpdateChannel(t *testing.T) {
	t.Parallel()
	var tests = []struct {
		name            string
		channel         string
		expectedChannel string
	}{
		{
			name:            "default",
			expectedChannel: Stable.String(),
		},
		{
			name:            "alpha",
			channel:         Alpha.String(),
			expectedChannel: Alpha.String(),
		},
		{
			name:            "beta",
			channel:         Beta.String(),
			expectedChannel: Beta.String(),
		},
		{
			name:            "nightly",
			channel:         Nightly.String(),
			expectedChannel: Nightly.String(),
		},
		{
			name:            "invalid",
			channel:         "not-a-real-channel",
			expectedChannel: Stable.String(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expectedChannel, SanitizeUpdateChannel(tt.channel))
		})
	}
}

// windowsAddExe appends ".exe" to the input string when running on Windows
func windowsAddExe(in string) string {
	if runtime.GOOS == "windows" {
		return in + ".exe"
	}

	return in
}
