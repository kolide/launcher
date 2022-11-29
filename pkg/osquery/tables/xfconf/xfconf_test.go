//go:build !windows
// +build !windows

package xfconf

import (
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/fsutil"
	"github.com/stretchr/testify/require"
)

func Test_getUserConfig(t *testing.T) {
	t.Parallel()

	setUpConfigFiles(t)

	xfconf := XfconfQuerier{
		logger: log.NewNopLogger(),
	}

	testUsername := "testUser"
	rowData := map[string]string{"username": testUsername}

	// Get the config without error
	config, err := xfconf.getUserConfig(&user.User{Username: testUsername}, "*", rowData)
	require.NoError(t, err, "expected no error fetching xfconf config")

	// Confirm we have some data in the config and that it looks correct
	require.Greater(t, len(config), 0)
	for _, configRow := range config {
		require.Equalf(t, testUsername, configRow["username"], "unexpected username: %s", configRow["username"])
		require.Truef(t, (configRow["channel"] == "xfce4-session" || configRow["channel"] == "xfce4-power-manager" || configRow["channel"] == "thunar-volman"), "unexpected channel: %s", configRow["channel"])

		if configRow["channel"] == "xfce4-power-manager" && configRow["fullkey"] == "channel/property/property/1/-value" {
			require.Equal(t, "true", configRow["value"], "default settings for power manager not overridden by user settings")
		}
	}
}

func setUpConfigFiles(t *testing.T) {
	// Make a temporary directory for default config, put config files there
	tmpDefaultDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDefaultDir, xfconfChannelXmlPath), 0755), "error making temp directory")
	fsutil.CopyFile(filepath.Join("testdata", "xfce4-session.xml"), filepath.Join(tmpDefaultDir, xfconfChannelXmlPath, "xfce4-session.xml"))
	fsutil.CopyFile(filepath.Join("testdata", "xfce4-power-manager-default.xml"), filepath.Join(tmpDefaultDir, xfconfChannelXmlPath, "xfce4-power-manager.xml"))

	// Set the environment variable for the default directory
	os.Setenv("XDG_CONFIG_DIRS", tmpDefaultDir)

	// Make a temporary directory for user-specific config, put config files there
	tmpUserDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmpUserDir, xfconfChannelXmlPath), 0755), "error making temp directory")
	fsutil.CopyFile(filepath.Join("testdata", "xfce4-power-manager.xml"), filepath.Join(tmpUserDir, xfconfChannelXmlPath, "xfce4-power-manager.xml"))
	fsutil.CopyFile(filepath.Join("testdata", "thunar-volman.xml"), filepath.Join(tmpUserDir, xfconfChannelXmlPath, "thunar-volman.xml"))

	// Set the environment variable for the user config directory
	os.Setenv("XDG_CONFIG_HOME", tmpUserDir)
}
