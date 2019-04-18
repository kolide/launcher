package main

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kolide/kit/stringutil"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/stretchr/testify/require"
)

// TestOptionsFromFlags isn't parallel to ensure that we don't pollute the environment
func TestOptionsFromFlags(t *testing.T) {
	os.Clearenv()

	testArgs, expectedOpts := getArgsAndResponse()

	testFlags := []string{}
	for k, v := range testArgs {
		testFlags = append(testFlags, k)
		if v != "" {
			testFlags = append(testFlags, v)
		}
	}

	opts, err := parseOptions(testFlags)
	require.NoError(t, err)
	require.Equal(t, expectedOpts, opts)
}

func TestOptionsFromEnv(t *testing.T) {
	os.Clearenv()

	testArgs, expectedOpts := getArgsAndResponse()

	for k, val := range testArgs {
		if val == "" {
			val = "true"
		}
		name := fmt.Sprintf("KOLIDE_LAUNCHER_%s", strings.ToUpper(strings.TrimLeft(k, "-")))
		require.NoError(t, os.Setenv(name, val))
	}
	opts, err := parseOptions([]string{})
	require.NoError(t, err)
	require.Equal(t, expectedOpts, opts)
}

func TestOptionsFromFile(t *testing.T) {
	os.Clearenv()

	testArgs, expectedOpts := getArgsAndResponse()

	flagFile, err := ioutil.TempFile("", "flag-file")
	require.NoError(t, err)
	defer os.Remove(flagFile.Name())

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

	opts, err := parseOptions([]string{"-config", flagFile.Name()})
	require.NoError(t, err)
	require.Equal(t, expectedOpts, opts)
}

func getArgsAndResponse() (map[string]string, *launcher.Options) {
	randomHostname := fmt.Sprintf("%s.example.com", stringutil.RandomString(8))
	randomInt := rand.Intn(1024)

	// includes both `-` and `--` for variety.
	args := map[string]string{
		"-control":             "", // This is a bool, it's special cased in the test routines
		"--hostname":           randomHostname,
		"-autoupdate_interval": "48h",
		"-logging_interval":    fmt.Sprintf("%ds", randomInt),
		"-osqueryd_path":       "/dev/null",
		"-transport":           "grpc",
	}

	opts := &launcher.Options{
		Control:            true,
		OsquerydPath:       "/dev/null",
		KolideServerURL:    randomHostname,
		GetShellsInterval:  3 * time.Second,
		LoggingInterval:    time.Duration(randomInt) * time.Second,
		AutoupdateInterval: 48 * time.Hour,
		NotaryServerURL:    "https://notary.kolide.co",
		MirrorServerURL:    "https://dl.kolide.co",
		UpdateChannel:      "stable",
		Transport:          "grpc",
	}

	return args, opts
}
