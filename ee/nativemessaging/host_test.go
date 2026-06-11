package nativemessaging

import (
	"testing"

	"github.com/kolide/launcher/v2/pkg/launcher"
	"github.com/stretchr/testify/require"
)

func Test_extractIdentifierFromExecutable(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName       string
		executablePath     string
		expectedIdentifier string
	}{
		{
			testCaseName:       "darwin - install location - default",
			executablePath:     "/usr/local/kolide-k2/Kolide.app/Contents/MacOS",
			expectedIdentifier: launcher.DefaultLauncherIdentifier,
		},
		{
			testCaseName:       "darwin - install location - non-default",
			executablePath:     "/usr/local/kolide-test-k2/Kolide.app/Contents/MacOS",
			expectedIdentifier: "kolide-test-k2",
		},
		{
			testCaseName:       "darwin - autoupdate location - default",
			executablePath:     "/var/kolide-k2/k2device.kolide.com/updates/launcher/2.2.2/Kolide.app/Contents/MacOS/launcher",
			expectedIdentifier: launcher.DefaultLauncherIdentifier,
		},
		{
			testCaseName:       "darwin - autoupdate location - non-default",
			executablePath:     "/var/kolide-test-k2/k2device.kolide.com/updates/launcher/2.2.2/Kolide.app/Contents/MacOS/launcher",
			expectedIdentifier: "kolide-test-k2",
		},
		{
			testCaseName:       "darwin - localdev location - non-default",
			executablePath:     "/User/kolide-engineer/Repos/launcher/build/launcher",
			expectedIdentifier: "kolide-nababe-k2",
		},
		{
			testCaseName:       "linux - install location - default",
			executablePath:     "/usr/local/kolide-k2/bin/launcher",
			expectedIdentifier: launcher.DefaultLauncherIdentifier,
		},
		{
			testCaseName:       "linux - install location - non-default",
			executablePath:     "/usr/local/kolide-test-k2/bin/launcher",
			expectedIdentifier: "kolide-test-k2",
		},
		{
			testCaseName:       "linux - autoupdate location - default",
			executablePath:     "/var/kolide-k2/k2device.kolide.com/updates/launcher/2.2.2/launcher",
			expectedIdentifier: launcher.DefaultLauncherIdentifier,
		},
		{
			testCaseName:       "linux - autoupdate location - non-default",
			executablePath:     "/var/kolide-test-k2/k2device.kolide.com/updates/launcher/2.2.2/launcher",
			expectedIdentifier: "kolide-test-k2",
		},
		{
			testCaseName:       "windows - install location - default",
			executablePath:     "C:\\Program Files\\Kolide\\Launcher-kolide-k2\\bin\\launcher.exe",
			expectedIdentifier: launcher.DefaultLauncherIdentifier,
		},
		{
			testCaseName:       "windows - install location - non-default",
			executablePath:     "C:\\Program Files\\Kolide\\Launcher-kolide-test-k2\\bin\\launcher.exe",
			expectedIdentifier: "kolide-test-k2",
		},
		{
			testCaseName:       "windows - autoupdate location - default",
			executablePath:     "C:\\ProgramData\\Kolide\\Launcher-kolide-k2\\data\\updates\\launcher\\2.2.2\\launcher.exe",
			expectedIdentifier: launcher.DefaultLauncherIdentifier,
		},
		{
			testCaseName:       "windows - autoupdate location - non-default",
			executablePath:     "C:\\ProgramData\\Kolide\\Launcher-kolide-test-k2\\data\\updates\\launcher\\2.2.2\\launcher.exe",
			expectedIdentifier: "kolide-test-k2",
		},
		{
			testCaseName:       "windows - autoupdate location, legacy root directory - default",
			executablePath:     "C:\\Program Files\\Kolide\\Launcher-kolide-k2\\data\\updates\\launcher\\2.2.2\\launcher.exe",
			expectedIdentifier: launcher.DefaultLauncherIdentifier,
		},
		{
			testCaseName:       "windows - autoupdate location, legacy root directory - non-default",
			executablePath:     "C:\\Program Files\\Kolide\\Launcher-kolide-test-k2\\data\\updates\\launcher\\2.2.2\\launcher.exe",
			expectedIdentifier: "kolide-test-k2",
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.expectedIdentifier, extractIdentifierFromExecutable(tt.executablePath))
		})
	}
}
