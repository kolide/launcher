package nativemessaging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_writeManifest_removeManifest(t *testing.T) {
	t.Parallel()

	identifier := "kolide-test-k2"
	hostName := nativeMessagingHostName(identifier)
	rootDir := t.TempDir()
	chromeM, firefoxM, err := buildManifests(hostName)
	require.NoError(t, err)

	for _, tt := range []struct {
		testCaseName         string
		expectedManifestPath string
		manifestToWrite      any
	}{
		{
			testCaseName:         "chrome",
			expectedManifestPath: launcherChromeManifestFilePath(rootDir),
			manifestToWrite:      chromeM,
		},
		{
			testCaseName:         "firefox",
			expectedManifestPath: launcherFirefoxManifestFilePath(rootDir),
			manifestToWrite:      firefoxM,
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			manifestRegistrationPaths := make([]string, 0)
			switch runtime.GOOS {
			case "windows":
				// Only one registry key on Windows
				manifestRegistrationPaths = append(manifestRegistrationPaths, `SOFTWARE\Kolide\Launcher\Test_writeManifest_removeManifest\NativeMessagingHosts\`+tt.testCaseName+`\`+hostName)
			default:
				// Multiple paths on macOS/Linux
				manifestRegistrationPaths = append(manifestRegistrationPaths,
					filepath.Join(rootDir, fmt.Sprintf("%s_%s.json", tt.testCaseName, hostName)),
					filepath.Join(rootDir, "some", "deeper", "path", fmt.Sprintf("%s_for_testing_%s.json", tt.testCaseName, hostName)), // tests having to make directories on the fly
				)
			}

			require.NoError(t, writeManifest(tt.manifestToWrite, tt.expectedManifestPath, manifestRegistrationPaths))

			// Confirm valid data written to expected path
			currentExe, err := os.Executable()
			require.NoError(t, err)
			_, err = os.Stat(tt.expectedManifestPath)
			require.NoError(t, err)

			fileContents, err := os.ReadFile(tt.expectedManifestPath)
			require.NoError(t, err)

			var currentManifest manifest
			require.NoError(t, json.Unmarshal(fileContents, &currentManifest))

			require.Equal(t, hostName, currentManifest.Name)
			require.Equal(t, nativeMessagingHostDescription, currentManifest.Description)
			require.Equal(t, currentExe, currentManifest.Path)
			require.Equal(t, nativeMessagingInterfaceType, currentManifest.Type)

			// Check existence of symlinks on macOS/Linux (registry key for Windows tested separately in Test_registerManifestFileLocation)
			if runtime.GOOS != "windows" {
				// Resolve the expected path (so that instead of `/var/...` on macOS we have `/private/var/...`)
				resolvedExpectedManifestPath, err := filepath.EvalSymlinks(tt.expectedManifestPath)
				require.NoError(t, err)
				for _, manifestRegistrationPath := range manifestRegistrationPaths {
					fi, err := os.Lstat(manifestRegistrationPath)
					require.NoError(t, err)

					// Confirm it's a symlink, pointing at the correct file
					require.NotEqual(t, 0, fi.Mode()&os.ModeSymlink)
					resolvedPath, err := filepath.EvalSymlinks(manifestRegistrationPath)
					require.NoError(t, err)
					require.Equal(t, resolvedExpectedManifestPath, resolvedPath)
				}
			}

			// Confirm no error on re-writing manifest
			require.NoError(t, writeManifest(tt.manifestToWrite, tt.expectedManifestPath, manifestRegistrationPaths))

			// Now, remove the manifest
			require.NoError(t, removeManifest(tt.expectedManifestPath, manifestRegistrationPaths))

			// Confirm manifest file is gone
			_, err = os.Stat(tt.expectedManifestPath)
			require.Error(t, err)
			require.True(t, os.IsNotExist(err))

			// Check that symlinks are gone on macOS/Linux
			if runtime.GOOS != "windows" {
				for _, manifestRegistrationPath := range manifestRegistrationPaths {
					_, err := os.Lstat(manifestRegistrationPath)
					require.Error(t, err)
					require.True(t, os.IsNotExist(err))
				}
			}
		})
	}
}
