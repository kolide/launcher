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

func Test_writeManifestToPaths(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	manifestPaths := make([]string, 0)
	switch runtime.GOOS {
	case "windows":
		// Only one path on Windows
		manifestPaths = append(manifestPaths, filepath.Join(rootDir, "nmh-manifest.json"))
	default:
		// Three paths on macOS
		manifestPaths = append(manifestPaths,
			filepath.Join(rootDir, fmt.Sprintf("chrome_%s.json", nativeMessagingHostName)),
			filepath.Join(rootDir, fmt.Sprintf("chrome_for_testing_%s.json", nativeMessagingHostName)),
			filepath.Join(rootDir, fmt.Sprintf("chromium_%s.json", nativeMessagingHostName)),
		)
	}

	testRegistryKey := `SOFTWARE\Kolide\Launcher\Test_writeManifestToPaths\Google\Chrome\NativeMessagingHosts\` + nativeMessagingHostName

	require.NoError(t, writeManifestToPaths(manifestPaths, testRegistryKey))

	// Confirm valid data written to each path
	currentExe, err := os.Executable()
	require.NoError(t, err)
	for _, manifestPath := range manifestPaths {
		_, err := os.Stat(manifestPath)
		require.NoError(t, err)

		fileContents, err := os.ReadFile(manifestPath)
		require.NoError(t, err)

		var currentManifest manifest
		require.NoError(t, json.Unmarshal(fileContents, &currentManifest))

		require.Equal(t, nativeMessagingHostName, currentManifest.Name)
		require.Equal(t, nativeMessagingHostDescription, currentManifest.Description)
		require.Equal(t, currentExe, currentManifest.Path)
		require.Equal(t, nativeMessagingInterfaceType, currentManifest.Type)
		require.Less(t, 0, len(currentManifest.AllowedOrigins))
	}
}
