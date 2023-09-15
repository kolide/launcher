package tuf

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_executableLocation(t *testing.T) {
	t.Parallel()

	updateDir := t.TempDir()

	var expectedOsquerydLocation string
	var expectedLauncherLocation string
	switch runtime.GOOS {
	case "darwin":
		expectedOsquerydLocation = filepath.Join(updateDir, "osquery.app", "Contents", "MacOS", "osqueryd")
		expectedLauncherLocation = filepath.Join(updateDir, "Kolide.app", "Contents", "MacOS", "launcher")
	case "windows":
		expectedOsquerydLocation = filepath.Join(updateDir, "osqueryd.exe")
		expectedLauncherLocation = filepath.Join(updateDir, "launcher.exe")
	case "linux":
		expectedOsquerydLocation = filepath.Join(updateDir, "osqueryd")
		expectedLauncherLocation = filepath.Join(updateDir, "launcher")
	}

	require.NoError(t, os.MkdirAll(filepath.Dir(expectedOsquerydLocation), 0755))
	f, err := os.Create(expectedOsquerydLocation)
	require.NoError(t, err)
	f.Close()

	osquerydLocation := executableLocation(updateDir, "osqueryd")
	require.Equal(t, expectedOsquerydLocation, osquerydLocation)

	launcherLocation := executableLocation(updateDir, "launcher")
	require.Equal(t, expectedLauncherLocation, launcherLocation)
}

func Test_executableLocation_nonAppBundle(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "darwin" {
		t.SkipNow()
	}

	updateDir := t.TempDir()
	expectedOsquerydLocation := filepath.Join(updateDir, "osqueryd")

	f, err := os.Create(expectedOsquerydLocation)
	require.NoError(t, err)
	f.Close()

	osquerydLocation := executableLocation(updateDir, "osqueryd")
	require.Equal(t, expectedOsquerydLocation, osquerydLocation)
}
