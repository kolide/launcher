package autoupdate

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewTufAutoupdater(t *testing.T) {
	t.Parallel()

	binaryPath := "some/path/to/launcher"
	testRootDir := t.TempDir()

	_, err := NewTufAutoupdater("https://tuf-devel.kolide.com", "https://dl.kolide.co", binaryPath, testRootDir)
	require.NoError(t, err, "could not initialize new TUF autoupdater")

	_, err = os.Stat(filepath.Join(testRootDir, "launcher-tuf-new"))
	require.NoError(t, err, "could not stat TUF directory that should have been initialized in test")

	_, err = os.Stat(filepath.Join(testRootDir, "launcher-tuf-new", "root.json"))
	require.NoError(t, err, "could not stat root.json that should have been created in test")
}

func TestRun(t *testing.T) {
	t.Parallel()
	t.SkipNow()
}

func TestRollingErrorCount(t *testing.T) {
	t.Parallel()
	t.SkipNow()
}

func Test_stop(t *testing.T) {
	t.Parallel()

	autoupdater := &TufAutoupdater{
		interrupt:     make(chan struct{}),
		checkInterval: 60 * time.Second,
	}

	stop, err := autoupdater.Run()
	require.NoError(t, err, "unexpected error when running")

	// Listen on interrupt channel to confirm that the interrupt is sent
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		<-autoupdater.interrupt
		wg.Done()
	}()
	time.Sleep(5 * time.Millisecond)

	// Shut down autoupdater
	stop()

	// Will time out if interrupt is not received
	wg.Wait()
}

func Test_versionFromTarget(t *testing.T) {
	t.Parallel()

	testLauncherVersions := []struct {
		target          string
		binary          string
		operatingSystem string
		version         string
	}{
		{
			target:          "launcher/darwin/launcher-0.10.1.tar.gz",
			binary:          "launcher",
			operatingSystem: "darwin",
			version:         "0.10.1",
		},
		{
			target:          "launcher/windows/launcher-1.13.5.tar.gz",
			binary:          "launcher.exe",
			operatingSystem: "windows",
			version:         "1.13.5",
		},
		{
			target:          "launcher/linux/launcher-0.13.5-40-gefdc582.tar.gz",
			binary:          "launcher",
			operatingSystem: "linux",
			version:         "0.13.5-40-gefdc582",
		},
		{
			target:          "osqueryd/darwin/osqueryd-5.8.1.tar.gz",
			binary:          "osqueryd",
			operatingSystem: "darwin",
			version:         "5.8.1",
		},
		{
			target:          "osqueryd/windows/osqueryd-0.8.1.tar.gz",
			binary:          "osqueryd.exe",
			operatingSystem: "windows",
			version:         "0.8.1",
		},
		{
			target:          "osqueryd/linux/osqueryd-5.8.2.tar.gz",
			binary:          "osqueryd",
			operatingSystem: "linux",
			version:         "5.8.2",
		},
	}

	for _, testLauncherVersion := range testLauncherVersions {
		autoupdater := &TufAutoupdater{
			binary:          testLauncherVersion.binary,
			operatingSystem: testLauncherVersion.operatingSystem,
		}
		require.Equal(t, testLauncherVersion.version, autoupdater.versionFromTarget(testLauncherVersion.target))
	}
}
