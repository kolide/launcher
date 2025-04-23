//go:build !windows
// +build !windows

// disabling on windows because for some reason the test cannot get access to the windows pipe it fails with:
// however it's just the test, works when using interactive mode on windows
// open \\.\pipe\kolide-osquery-.....: The system cannot find the file specified.
package interactive

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/kolide/kit/fsutil"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	agentsqlite "github.com/kolide/launcher/ee/agent/storage/sqlite"
	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/packaging"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var osquerydCacheDir = filepath.Join(os.TempDir(), "launcher_interactive_tests")

func TestMain(m *testing.M) {
	// download and cache the osquerd binary before tests run
	target := packaging.Target{}
	if err := target.PlatformFromString(runtime.GOOS); err != nil {
		fmt.Printf("error parsing platform: %s, %s", err, runtime.GOOS)
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit in tests
	}
	target.Arch = packaging.ArchFlavor(runtime.GOARCH)
	if runtime.GOOS == "darwin" {
		target.Arch = packaging.Universal
	}

	if err := os.MkdirAll(osquerydCacheDir, fsutil.DirMode); err != nil {
		fmt.Printf("error creating cache dir: %s", err)
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit in tests
	}

	_, err := packaging.FetchBinary(context.TODO(), osquerydCacheDir, "osqueryd", target.PlatformBinaryName("osqueryd"), "stable", target)
	if err != nil {
		fmt.Printf("error fetching binary osqueryd binary: %s", err)
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit in tests
	}

	// Run the tests!
	retCode := m.Run()
	os.Exit(retCode) //nolint:forbidigo // Fine to use os.Exit in tests
}

// TestProc tests the start process function, it's named weird because path of the temp dir has to be short enough
// to not exceed the max number of charcters for the socket path.
func TestProc(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		useShortRootDir bool
		osqueryFlags    []string
		wantProc        bool
		errContainsStr  string
	}{
		{
			name:            "no flags",
			useShortRootDir: true,
			wantProc:        true,
		},
		{
			name:            "flags",
			useShortRootDir: true,
			osqueryFlags: []string{
				"verbose",
				"force=false",
			},
			wantProc: true,
		},
		{
			name:            "config path",
			useShortRootDir: true,
			osqueryFlags: []string{
				fmt.Sprintf("config_path=%s", ulid.New()),
			},
			wantProc: true,
		},
		{
			name:            "socket path too long, the name of the test causes the socket path to be to long to be created, resulting in timeout waiting for the socket",
			useShortRootDir: false,
			wantProc:        false,
			errContainsStr:  "error waiting for osquery to create socket",
		},
	}
	for _, tt := range tests { //nolint:paralleltest
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var rootDir string
			if tt.useShortRootDir {
				rootDir = testRootDirectory(t)
			} else {
				rootDir = t.TempDir()
			}

			require.NoError(t, downloadOsquery(rootDir))

			var logBytes threadsafebuffer.ThreadSafeBuffer
			slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
				AddSource: true,
				Level:     slog.LevelDebug,
			}))

			// Set up knapsack
			mockSack := mocks.NewKnapsack(t)
			mockSack.On("OsquerydPath").Return(filepath.Join(rootDir, "osqueryd"))
			mockSack.On("OsqueryFlags").Return(tt.osqueryFlags)
			mockSack.On("Slogger").Return(slogger)
			mockSack.On("RootDirectory").Maybe().Return(rootDir)
			store, err := storageci.NewStore(t, slogger, storage.KatcConfigStore.String())
			require.NoError(t, err)
			mockSack.On("KatcConfigStore").Return(store)
			mockSack.On("TableGenerateTimeout").Return(4 * time.Minute).Maybe()
			mockSack.On("RegisterChangeObserver", mock.Anything, keys.TableGenerateTimeout).Return().Maybe()

			// Set up the startup settings store -- opening RW ensures that the db exists
			// with the appropriate migrations.
			startupSettingsStore, err := agentsqlite.OpenRW(context.TODO(), rootDir, agentsqlite.StartupSettingsStore)
			require.NoError(t, err, "initializing startup settings store")
			require.NoError(t, startupSettingsStore.Close())

			// Make sure the process starts in a timely fashion
			var proc *os.Process
			startErr := make(chan error)
			go func() {
				proc, _, err = StartProcess(mockSack, rootDir)
				startErr <- err
			}()

			select {
			case err := <-startErr:
				if tt.errContainsStr != "" {
					require.Error(t, err, fmt.Sprintf("logs: %s", logBytes.String()))
					require.Contains(t, err.Error(), tt.errContainsStr)
				} else {
					require.NoError(t, err, fmt.Sprintf("logs: %s", logBytes.String()))
				}
			case <-time.After(2 * time.Minute):
				t.Error("process did not start before timeout", fmt.Sprintf("logs: %s", logBytes.String()))
				t.FailNow()
			}

			if tt.wantProc {
				require.NotNil(t, proc, fmt.Sprintf("logs: %s", logBytes.String()))

				// Wait until proc exits
				procExitErr := make(chan error)
				go func() {
					_, err := proc.Wait()
					procExitErr <- err
				}()

				select {
				case err := <-procExitErr:
					require.NoError(t, err, fmt.Sprintf("logs: %s", logBytes.String()))
				case <-time.After(2 * time.Minute):
					t.Error("process did not exit before timeout", fmt.Sprintf("logs: %s", logBytes.String()))
					t.FailNow()
				}
			} else {
				require.Nil(t, proc)
			}
		})
	}
}

func downloadOsquery(dir string) error {
	target := packaging.Target{}
	if err := target.PlatformFromString(runtime.GOOS); err != nil {
		return fmt.Errorf("error parsing platform: %w, %s", err, runtime.GOOS)
	}

	// Binary is already downloaded to osquerydCacheDir -- create a symlink at outputFile,
	// rather than copying the file, to maybe avoid https://github.com/golang/go/issues/22315
	outputFile := filepath.Join(dir, "osqueryd")
	sourceFile := filepath.Join(osquerydCacheDir, fmt.Sprintf("osqueryd-%s-stable", runtime.GOOS), "osqueryd")
	if err := os.Symlink(sourceFile, outputFile); err != nil {
		return fmt.Errorf("creating symlink from %s to %s: %w", sourceFile, outputFile, err)
	}

	return nil
}

// testRootDirectory returns a temporary directory suitable for use in these tests.
// The default t.TempDir is too long of a path, creating too long of an osquery
// extension socket, on posix systems.
func testRootDirectory(t *testing.T) string {
	for i := 0; i < 100; i++ {
		ulid := ulid.New()
		rootDir := filepath.Join(os.TempDir(), ulid[len(ulid)-4:])

		// Make sure root dir doesn't already exist
		if _, err := os.Stat(rootDir); err == nil {
			// Root dir exists, try again
			continue
		}

		require.NoError(t, os.Mkdir(rootDir, 0700))
		t.Cleanup(func() {
			if err := os.RemoveAll(rootDir); err != nil {
				t.Errorf("testRootDirectory RemoveAll cleanup: %v", err)
			}
		})

		return rootDir
	}

	t.Error("failed to make new unique root directory in tmp")
	t.FailNow()
	return ""
}
