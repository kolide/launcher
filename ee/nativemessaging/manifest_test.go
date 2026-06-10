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
	manifestRegistrationPaths := make([]string, 0)
	switch runtime.GOOS {
	case "windows":
		// Only one registry key on Windows
		manifestRegistrationPaths = append(manifestRegistrationPaths, `SOFTWARE\Kolide\Launcher\Test_writeManifest_removeManifest\Google\Chrome\NativeMessagingHosts\`+hostName)
	default:
		// Multiple paths on macOS/Linux
		manifestRegistrationPaths = append(manifestRegistrationPaths,
			filepath.Join(rootDir, fmt.Sprintf("chrome_%s.json", hostName)),
			filepath.Join(rootDir, "some", "deeper", "path", fmt.Sprintf("chrome_for_testing_%s.json", hostName)), // tests having to make directories on the fly
			filepath.Join(rootDir, fmt.Sprintf("chromium_%s.json", hostName)),
		)
	}

	expectedManifestPath := launcherManifestFilePath(rootDir)
	require.NoError(t, writeManifest(expectedManifestPath, manifestRegistrationPaths, hostName))

	// Confirm valid data written to expected path
	currentExe, err := os.Executable()
	require.NoError(t, err)
	_, err = os.Stat(expectedManifestPath)
	require.NoError(t, err)

	fileContents, err := os.ReadFile(expectedManifestPath)
	require.NoError(t, err)

	var currentManifest manifest
	require.NoError(t, json.Unmarshal(fileContents, &currentManifest))

	require.Equal(t, hostName, currentManifest.Name)
	require.Equal(t, nativeMessagingHostDescription, currentManifest.Description)
	require.Equal(t, currentExe, currentManifest.Path)
	require.Equal(t, nativeMessagingInterfaceType, currentManifest.Type)
	require.Less(t, 0, len(currentManifest.AllowedOrigins))

	// Check existence of symlinks on macOS/Linux (registry key for Windows tested separately in Test_registerManifestFileLocation)
	if runtime.GOOS != "windows" {
		// Resolve the expected path (so that instead of `/var/...` on macOS we have `/private/var/...`)
		resolvedExpectedManifestPath, err := filepath.EvalSymlinks(expectedManifestPath)
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
	require.NoError(t, writeManifest(expectedManifestPath, manifestRegistrationPaths, hostName))

	// Now, remove the manifest
	require.NoError(t, removeManifest(expectedManifestPath, manifestRegistrationPaths))

	// Confirm manifest file is gone
	_, err = os.Stat(expectedManifestPath)
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
}
