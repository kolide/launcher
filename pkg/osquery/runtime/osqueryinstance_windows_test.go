//go:build windows
// +build windows

package runtime

import (
	"strings"
	"testing"

	"github.com/kolide/launcher/ee/agent/types"
	typesMocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	settingsstoremock "github.com/kolide/launcher/pkg/osquery/mocks"
	"github.com/stretchr/testify/require"
)

func TestCreateOsqueryCommandEnvVars(t *testing.T) {
	t.Parallel()

	k := typesMocks.NewKnapsack(t)
	k.On("WatchdogEnabled").Return(true)
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)
	k.On("OsqueryVerbose").Return(true)
	k.On("OsqueryFlags").Return([]string{})
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("RootDirectory").Return("")
	setupHistory(t, k)

	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient(t), settingsstoremock.NewSettingsStoreWriter(t))
	i.paths = &osqueryFilePaths{
		pidfilePath:           "/foo/bar/osquery-abcd.pid",
		databasePath:          "/foo/bar/osquery.db",
		extensionSocketPath:   "/foo/bar/osquery.sock",
		extensionAutoloadPath: "/foo/bar/osquery.autoload",
	}

	cmd, err := i.createOsquerydCommand("") // we do not actually exec so don't need to download a real osquery for this test
	require.NoError(t, err)

	systemDriveEnvVarFound := false
	for _, envVar := range cmd.Env {
		if strings.Contains(envVar, "SystemDrive") {
			systemDriveEnvVarFound = true
			break
		}
	}

	require.True(t, systemDriveEnvVarFound, "SystemDrive env var missing from command")

	k.AssertExpectations(t)
}
