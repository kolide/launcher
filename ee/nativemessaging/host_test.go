package nativemessaging

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kolide/launcher/v2/pkg/launcher"
	"github.com/stretchr/testify/require"
)

func Test_extractIdentifierFromExecutable(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName       string
		platform           string
		executablePath     string
		expectedIdentifier string
	}{
		{
			testCaseName:       "darwin - install location - default",
			platform:           "darwin",
			executablePath:     "/usr/local/kolide-k2/Kolide.app/Contents/MacOS",
			expectedIdentifier: launcher.DefaultLauncherIdentifier,
		},
		{
			testCaseName:       "darwin - install location - non-default",
			platform:           "darwin",
			executablePath:     "/usr/local/kolide-test-k2/Kolide.app/Contents/MacOS",
			expectedIdentifier: "kolide-test-k2",
		},
		{
			testCaseName:       "darwin - autoupdate location - default",
			platform:           "darwin",
			executablePath:     "/var/kolide-k2/k2device.kolide.com/updates/launcher/2.2.2/Kolide.app/Contents/MacOS/launcher",
			expectedIdentifier: launcher.DefaultLauncherIdentifier,
		},
		{
			testCaseName:       "darwin - autoupdate location - non-default",
			platform:           "darwin",
			executablePath:     "/var/kolide-test-k2/k2device.kolide.com/updates/launcher/2.2.2/Kolide.app/Contents/MacOS/launcher",
			expectedIdentifier: "kolide-test-k2",
		},
		{
			testCaseName:       "darwin - localdev location - non-default",
			platform:           "darwin",
			executablePath:     "/User/kolide-engineer/Repos/launcher/build/launcher",
			expectedIdentifier: "kolide-nababe-k2",
		},
		{
			testCaseName:       "linux - install location - default",
			platform:           "linux",
			executablePath:     "/usr/local/kolide-k2/bin/launcher",
			expectedIdentifier: launcher.DefaultLauncherIdentifier,
		},
		{
			testCaseName:       "linux - install location - non-default",
			platform:           "linux",
			executablePath:     "/usr/local/kolide-test-k2/bin/launcher",
			expectedIdentifier: "kolide-test-k2",
		},
		{
			testCaseName:       "linux - autoupdate location - default",
			platform:           "linux",
			executablePath:     "/var/kolide-k2/k2device.kolide.com/updates/launcher/2.2.2/launcher",
			expectedIdentifier: launcher.DefaultLauncherIdentifier,
		},
		{
			testCaseName:       "linux - autoupdate location - non-default",
			platform:           "linux",
			executablePath:     "/var/kolide-test-k2/k2device.kolide.com/updates/launcher/2.2.2/launcher",
			expectedIdentifier: "kolide-test-k2",
		},
		{
			testCaseName:       "windows - install location - default",
			platform:           "windows",
			executablePath:     "C:\\Program Files\\Kolide\\Launcher-kolide-k2\\bin\\launcher.exe",
			expectedIdentifier: launcher.DefaultLauncherIdentifier,
		},
		{
			testCaseName:       "windows - install location - non-default",
			platform:           "windows",
			executablePath:     "C:\\Program Files\\Kolide\\Launcher-kolide-test-k2\\bin\\launcher.exe",
			expectedIdentifier: "kolide-test-k2",
		},
		{
			testCaseName:       "windows - autoupdate location - default",
			platform:           "windows",
			executablePath:     "C:\\ProgramData\\Kolide\\Launcher-kolide-k2\\data\\updates\\launcher\\2.2.2\\launcher.exe",
			expectedIdentifier: launcher.DefaultLauncherIdentifier,
		},
		{
			testCaseName:       "windows - autoupdate location - non-default",
			platform:           "windows",
			executablePath:     "C:\\ProgramData\\Kolide\\Launcher-kolide-test-k2\\data\\updates\\launcher\\2.2.2\\launcher.exe",
			expectedIdentifier: "kolide-test-k2",
		},
		{
			testCaseName:       "windows - autoupdate location, legacy root directory - default",
			platform:           "windows",
			executablePath:     "C:\\Program Files\\Kolide\\Launcher-kolide-k2\\data\\updates\\launcher\\2.2.2\\launcher.exe",
			expectedIdentifier: launcher.DefaultLauncherIdentifier,
		},
		{
			testCaseName:       "windows - autoupdate location, legacy root directory - non-default",
			platform:           "windows",
			executablePath:     "C:\\Program Files\\Kolide\\Launcher-kolide-test-k2\\data\\updates\\launcher\\2.2.2\\launcher.exe",
			expectedIdentifier: "kolide-test-k2",
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			if tt.platform != runtime.GOOS {
				return
			}

			require.Equal(t, tt.expectedIdentifier, extractIdentifierFromExecutable(tt.executablePath))
		})
	}
}

func Test_validateBrowserPath(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName string
		platform     string
		browserPath  string
		browserName  string
		shouldPass   bool
	}{
		{
			testCaseName: "darwin + chrome",
			platform:     "darwin",
			browserPath:  "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			browserName:  "Google Chrome",
			shouldPass:   true,
		},
		{
			testCaseName: "darwin + chrome beta",
			platform:     "darwin",
			browserPath:  "/Applications/Google Chrome Beta.app/Contents/MacOS/Google Chrome Beta",
			browserName:  "Google Chrome Beta",
			shouldPass:   true,
		},
		{
			testCaseName: "darwin + chrome dev",
			platform:     "darwin",
			browserPath:  "/Applications/Google Chrome Dev.app/Contents/MacOS/Google Chrome Dev",
			browserName:  "Google Chrome Dev",
			shouldPass:   true,
		},
		{
			testCaseName: "darwin + chrome canary",
			platform:     "darwin",
			browserPath:  "/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
			browserName:  "Google Chrome Canary",
			shouldPass:   true,
		},
		{
			testCaseName: "darwin + chromium",
			platform:     "darwin",
			browserPath:  "/Applications/Chromium.app/Contents/MacOS/Chromium",
			browserName:  "Chromium",
			shouldPass:   false, // inadequate codesigning
		},
		{
			testCaseName: "windows + chrome - Program Files",
			platform:     "windows",
			browserPath:  `C:\Program Files\Google\Chrome\Application\chrome.exe`,
			browserName:  "chrome.exe",
			shouldPass:   true,
		},
		{
			testCaseName: "windows + chrome - Program Files (x86)",
			platform:     "windows",
			browserPath:  `C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			browserName:  "chrome.exe",
			shouldPass:   true,
		},
		{
			testCaseName: "windows + chrome - per-user",
			platform:     "windows",
			browserPath:  filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
			browserName:  "chrome.exe",
			shouldPass:   true,
		},
		{
			testCaseName: "windows + chrome beta - Program Files",
			platform:     "windows",
			browserPath:  `C:\Program Files\Google\Chrome Beta\Application\chrome.exe`,
			browserName:  "chrome.exe",
			shouldPass:   true,
		},
		{
			testCaseName: "windows + chrome beta - Program Files (x86)",
			platform:     "windows",
			browserPath:  `C:\Program Files (x86)\Google\Chrome Beta\Application\chrome.exe`,
			browserName:  "chrome.exe",
			shouldPass:   true,
		},
		{
			testCaseName: "windows + chrome beta - per-user",
			platform:     "windows",
			browserPath:  filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome Beta", "Application", "chrome.exe"),
			browserName:  "chrome.exe",
			shouldPass:   true,
		},
		{
			testCaseName: "windows + chrome dev - Program Files",
			platform:     "windows",
			browserPath:  `C:\Program Files\Google\Chrome Dev\Application\chrome.exe`,
			browserName:  "chrome.exe",
			shouldPass:   true,
		},
		{
			testCaseName: "windows + chrome dev - Program Files (x86)",
			platform:     "windows",
			browserPath:  `C:\Program Files (x86)\Google\Chrome Dev\Application\chrome.exe`,
			browserName:  "chrome.exe",
			shouldPass:   true,
		},
		{
			testCaseName: "windows + chrome dev - per-user",
			platform:     "windows",
			browserPath:  filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome Dev", "Application", "chrome.exe"),
			browserName:  "chrome.exe",
			shouldPass:   true,
		},
		{
			testCaseName: "windows + chrome canary - per-user",
			platform:     "windows",
			browserPath:  filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome SxS", "Application", "chrome.exe"),
			browserName:  "chrome.exe",
			shouldPass:   true,
		},
		{
			testCaseName: "windows + chromium - Program Files",
			platform:     "windows",
			browserPath:  `C:\Program Files\Chromium\Application\chrome.exe`,
			browserName:  "chrome.exe",
			shouldPass:   false, // not signed by Google LLC
		},
		{
			testCaseName: "windows + chromium - per-user",
			platform:     "windows",
			browserPath:  filepath.Join(os.Getenv("LOCALAPPDATA"), "Chromium", "Application", "chrome.exe"),
			browserName:  "chrome.exe",
			shouldPass:   false, // not signed by Google LLC
		},
		{
			testCaseName: "linux + chrome",
			platform:     "linux",
			browserPath:  "/opt/google/chrome/chrome",
			browserName:  "chrome",
			shouldPass:   true,
		},
		{
			testCaseName: "linux + chrome beta",
			platform:     "linux",
			browserPath:  "/opt/google/chrome-beta/chrome",
			browserName:  "chrome",
			shouldPass:   true,
		},
		{
			testCaseName: "linux + chrome dev",
			platform:     "linux",
			browserPath:  "/opt/google/chrome-unstable/chrome",
			browserName:  "chrome",
			shouldPass:   true,
		},
		{
			testCaseName: "linux + chromium - snap",
			platform:     "linux",
			browserPath:  "/snap/chromium/current/usr/lib/chromium-browser/chrome",
			browserName:  "chrome",
			shouldPass:   true,
		},
		{
			testCaseName: "linux + chromium - deb",
			platform:     "linux",
			browserPath:  "/usr/lib/chromium-browser/chromium-browser",
			browserName:  "chromium-browser",
			shouldPass:   true,
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			if tt.platform != runtime.GOOS {
				return
			}
			// Check whether browser is actually installed -- in CI, almost certainly not
			if _, err := os.Stat(tt.browserPath); err != nil {
				return
			}

			validationErr := validateBrowser(t.Context(), tt.browserPath, tt.browserName)
			if tt.shouldPass {
				require.NoError(t, validationErr)
			} else {
				require.Error(t, validationErr)
			}
		})
	}
}
