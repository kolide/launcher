//go:build linux
// +build linux

package xfconf

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/fsutil"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/require"
)

func Test_getUserConfig(t *testing.T) {
	t.Parallel()

	setUpConfigFiles(t)

	xfconf := xfconfTable{
		logger: log.NewNopLogger(),
	}

	testUsername := "testUser"

	// Get the default config without error
	defaultConfig, err := xfconf.getDefaultConfig()
	require.NoError(t, err, "expected no error fetching default xfconfig")
	require.Greater(t, len(defaultConfig), 0)
	// Confirm lock-screen-suspend-hibernate is false now so we can validate that it got overridden after
	powerManagerChannel, ok := defaultConfig["channel/xfce4-power-manager"]
	require.True(t, ok, "invalid default data format -- missing channel")
	powerManagerProperties, ok := powerManagerChannel.(map[string]interface{})["xfce4-power-manager"]
	require.True(t, ok, "invalid default data format -- missing xfce4-power-manager property")
	lockScreenSuspendHibernate, ok := powerManagerProperties.(map[string]interface{})["lock-screen-suspend-hibernate"]
	require.True(t, ok, "invalid default data format -- missing lock-screen-suspend-hibernate property")
	require.Equal(t, "false", lockScreenSuspendHibernate)

	// Get the combined config without error
	config, err := xfconf.generateForUser(&user.User{Username: testUsername}, table.QueryContext{}, defaultConfig)
	require.NoError(t, err, "expected no error fetching xfconf config")

	// Confirm we have some data in the config and that it looks correct
	require.Greater(t, len(config), 0)
	for _, configRow := range config {
		// Confirm username was set correctly on all rows
		require.Equalf(t, testUsername, configRow["username"], "unexpected username: %s", configRow["username"])

		// Confirm each row came from an expected channel
		require.Truef(t, (strings.HasPrefix(configRow["fullkey"], "channel/xfce4-session") ||
			strings.HasPrefix(configRow["fullkey"], "channel/xfce4-power-manager") ||
			strings.HasPrefix(configRow["fullkey"], "channel/thunar-volman")),
			"unexpected channel: %s", configRow["fullkey"])

		// Confirm that we took user-specific config values over default ones
		if configRow["fullkey"] == "channel/xfce4-power-manager/xfce4-power-manager/lock-screen-suspend-hibernate" {
			require.Equal(t, "true", configRow["value"], "default settings for power manager not overridden by user settings")
		}
	}

	// Query with a constraint this time
	constraintList := table.ConstraintList{
		Affinity: table.ColumnTypeText,
		Constraints: []table.Constraint{
			{
				Operator:   table.OperatorEquals,
				Expression: "*/autoopen*",
			},
		},
	}
	q := table.QueryContext{
		Constraints: map[string]table.ConstraintList{
			"query": constraintList,
		},
	}
	constrainedConfig, err := xfconf.generateForUser(&user.User{Username: testUsername}, q, defaultConfig)
	require.NoError(t, err, "expected no error fetching xfconf config with query constraints")
	require.Equal(t, 1, len(constrainedConfig), "query wrong number of rows, expected exactly 1")
	require.Equal(t, "channel/thunar-volman/autoopen/enabled", constrainedConfig[0]["fullkey"], "query fetched wrong row")
	require.Equal(t, "false", constrainedConfig[0]["value"], "fetched incorrect value for autoopen enabled")
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

func Test_getUserConfig_SoftError(t *testing.T) {
	t.Parallel()

	setUpConfigFiles(t)

	xfconf := xfconfTable{
		logger: log.NewNopLogger(),
	}

	// Get the combined config without error
	constraintList := table.ConstraintList{
		Affinity: table.ColumnTypeText,
		Constraints: []table.Constraint{
			{
				Operator:   table.OperatorEquals,
				Expression: "AFakeUserThatDoesNotExist",
			},
		},
	}
	q := table.QueryContext{
		Constraints: map[string]table.ConstraintList{
			"username": constraintList,
		},
	}
	config, err := xfconf.generate(context.TODO(), q)
	require.NoError(t, err, "expected no error fetching xfconf config")
	require.Equal(t, 0, len(config), "expected no rows")
}
