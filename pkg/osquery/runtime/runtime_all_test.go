package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	typesMocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/kolide/launcher/pkg/packaging"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var testOsqueryBinaryDirectory string

// TestMain overrides the default test main function. This allows us to share setup/teardown.
func TestMain(m *testing.M) {
	binDirectory, rmBinDirectory, err := osqueryTempDir()
	if err != nil {
		fmt.Println("Failed to make temp dir for test binaries")
		os.Exit(1)
	}
	defer rmBinDirectory()

	s, err := storageci.NewStore(nil, multislogger.NewNopLogger(), storage.OsqueryHistoryInstanceStore.String())
	if err != nil {
		fmt.Println("Failed to make new store")
		os.Exit(1)
	}
	if err := history.InitHistory(s); err != nil {
		fmt.Println("Failed to init history")
		os.Exit(1)
	}

	testOsqueryBinaryDirectory = filepath.Join(binDirectory, "osqueryd")

	thrift.ServerConnectivityCheckInterval = 100 * time.Millisecond

	if err := downloadOsqueryInBinDir(binDirectory); err != nil {
		fmt.Printf("Failed to download osquery: %v\n", err)
		os.Exit(1)
	}

	// Run the tests!
	retCode := m.Run()
	os.Exit(retCode)
}

// downloadOsqueryInBinDir downloads osqueryd. This allows the test
// suite to run on hosts lacking osqueryd. We could consider moving this into a deps step.
func downloadOsqueryInBinDir(binDirectory string) error {
	target := packaging.Target{}
	if err := target.PlatformFromString(runtime.GOOS); err != nil {
		return fmt.Errorf("Error parsing platform: %s: %w", runtime.GOOS, err)
	}

	outputFile := filepath.Join(binDirectory, "osqueryd")
	if runtime.GOOS == "windows" {
		outputFile += ".exe"
	}

	cacheDir := "/tmp"
	if runtime.GOOS == "windows" {
		cacheDir = os.Getenv("TEMP")
	}

	path, err := packaging.FetchBinary(context.TODO(), cacheDir, "osqueryd", target.PlatformBinaryName("osqueryd"), "stable", target)
	if err != nil {
		return fmt.Errorf("An error occurred fetching the osqueryd binary: %w", err)
	}

	if err := fsutil.CopyFile(path, outputFile); err != nil {
		return fmt.Errorf("Couldn't copy file to %s: %w", outputFile, err)
	}

	return nil
}

// waitHealthy expects the instance to be healthy within 30 seconds, or else
// fatals the test
func waitHealthy(t *testing.T, runner *Runner) {
	require.NoError(t, backoff.WaitFor(func() error {
		if runner.Healthy() == nil {
			return nil
		}
		return fmt.Errorf("instance not healthy")
	}, 120*time.Second, 5*time.Second))
}

func TestSimplePath(t *testing.T) {
	t.Parallel()
	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)
	defer rmRootDirectory()

	k := typesMocks.NewKnapsack(t)
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("PinnedOsquerydVersion").Return("")

	runner := New(
		k,
		WithKnapsack(k),
		WithRootDirectory(rootDirectory),
		WithOsquerydBinary(testOsqueryBinaryDirectory),
	)

	go runner.Run()

	// never gets health on windows
	waitHealthy(t, runner)

	require.NotEmpty(t, runner.instance.stats.StartTime, "start time should be added to instance stats on start up")
	require.NotEmpty(t, runner.instance.stats.ConnectTime, "connect time should be added to instance stats on start up")

	require.NoError(t, runner.Shutdown())
}
