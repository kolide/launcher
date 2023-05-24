package main

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kolide/kit/stringutil"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/stretchr/testify/require"
)

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

	opts, err := parseOptions("", testFlags)
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
		require.NoError(t, os.Setenv(name, val))
	}
	opts, err := parseOptions("", []string{})
	require.NoError(t, err)
	require.Equal(t, expectedOpts, opts)
}

func TestOptionsFromFile(t *testing.T) { // nolint:paralleltest
	os.Clearenv()

	testArgs, expectedOpts := getArgsAndResponse()

	flagFile, err := os.CreateTemp("", "flag-file")
	require.NoError(t, err)
	defer os.Remove(flagFile.Name())
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

	opts, err := parseOptions("", []string{"-config", flagFile.Name()})
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
			opts, err := parseOptions("", tt.testFlags)
			require.NoError(t, err, "could not parse options")
			require.Equal(t, tt.expectedControlServer, opts.ControlServerURL, "incorrect control server")
			require.Equal(t, tt.expectedInsecureControlTLS, opts.InsecureControlTLS, "incorrect insecure TLS")
			require.Equal(t, tt.expectedDisableControlTLS, opts.DisableControlTLS, "incorrect disable control TLS")
		})
	}
}

func getArgsAndResponse() (map[string]string, *launcher.Options) {
	randomHostname := fmt.Sprintf("%s.example.com", stringutil.RandomString(8))
	randomInt := rand.Intn(1024)

	// includes both `-` and `--` for variety.
	args := map[string]string{
		"--hostname":            randomHostname,
		"-autoupdate_interval":  "48h",
		"-logging_interval":     fmt.Sprintf("%ds", randomInt),
		"-osqueryd_path":        windowsAddExe("/dev/null"),
		"-transport":            "grpc",
		"-autoloaded_extension": "some-extension.ext",
	}

	opts := &launcher.Options{
		AutoupdateInitialDelay: 1 * time.Hour,
		AutoupdateInterval:     48 * time.Hour,
		CompactDbMaxTx:         int64(65536),
		Control:                false,
		ControlServerURL:       "",
		ControlRequestInterval: 60 * time.Second,
		KolideServerURL:        randomHostname,
		LoggingInterval:        time.Duration(randomInt) * time.Second,
		MirrorServerURL:        "https://dl.kolide.co",
		NotaryPrefix:           "kolide",
		NotaryServerURL:        "https://notary.kolide.co",
		TufServerURL:           "https://tuf.kolide.com",
		OsquerydPath:           windowsAddExe("/dev/null"),
		Transport:              "grpc",
		UpdateChannel:          "stable",
		AutoloadedExtensions:   []string{"some-extension.ext"},
		DelayStart:             0 * time.Second,
	}

	return args, opts
}

func windowsAddExe(in string) string {
	if runtime.GOOS == "windows" {
		return in + ".exe"
	}

	return in
}
