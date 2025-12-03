//go:build !linux
// +build !linux

package interactive

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/storage/inmemory"
	agentsqlite "github.com/kolide/launcher/ee/agent/storage/sqlite"
	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/osquery/testutil"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var testOsqueryBinary string

// downloadOnceFunc downloads a real osquery binary for use in tests. This function
// can be called multiple times but will only execute once -- the osquery binary is
// stored at path `testOsqueryBinary` and can be reused by all subsequent tests.
var downloadOnceFunc = sync.OnceFunc(func() {
	testOsqueryBinary, _, _ = testutil.DownloadOsquery("nightly")
})

// copyBinary ensures we've downloaded a test osquery binary, then creates a symlink
// to it at the expected `executablePath` location. The cached binary is already signed,
// so the symlink will point to an executable binary.
func copyBinary(t *testing.T, executablePath string) {
	downloadOnceFunc()

	require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0755))
	require.NoError(t, os.Symlink(testOsqueryBinary, executablePath))
}

// TestProc tests the start process function, it's named weird because path of the temp dir has to be short enough
// to not exceed the max number of charcters for the socket path.
func TestProc(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name               string
		useShortRootDir    bool
		osqueryFlags       []string
		configFileContents []byte
		wantProc           bool
		errContainsStr     string
	}

	tests := []testCase{
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
			configFileContents: []byte(`
{
  "options": {
    "verbose": true
  }
}
`),
			wantProc: true,
		},
	}

	if runtime.GOOS != "windows" {
		tests = append(tests, testCase{
			name:            "socket path too long, the name of the test causes the socket path to be to long to be created, resulting in timeout waiting for the socket",
			useShortRootDir: false,
			wantProc:        false,
			errContainsStr:  "error waiting for osquery to create socket",
		},
		)
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

			osquerydPath := filepath.Join(rootDir, "osquery")
			copyBinary(t, osquerydPath)

			var logBytes threadsafebuffer.ThreadSafeBuffer
			slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
				AddSource: true,
				Level:     slog.LevelDebug,
			}))

			// Set up config file, if needed
			if len(tt.configFileContents) > 0 {
				configFilePath := filepath.Join(rootDir, "osquery.conf")
				require.NoError(t, os.WriteFile(configFilePath, tt.configFileContents, 0755), "writing config file")
				tt.osqueryFlags = append(tt.osqueryFlags, fmt.Sprintf("config_path=%s", configFilePath))
			}

			// Set up knapsack
			mockSack := mocks.NewKnapsack(t)
			mockSack.On("OsquerydPath").Return(filepath.Join(rootDir, "osquery"))
			mockSack.On("OsqueryFlags").Return(tt.osqueryFlags)
			mockSack.On("Slogger").Return(slogger)
			mockSack.On("RootDirectory").Maybe().Return(rootDir)
			store, err := storageci.NewStore(t, slogger, storage.KatcConfigStore.String())
			require.NoError(t, err)
			mockSack.On("KatcConfigStore").Return(store)
			mockSack.On("TableGenerateTimeout").Return(4 * time.Minute).Maybe()
			mockSack.On("RegisterChangeObserver", mock.Anything, keys.TableGenerateTimeout).Return().Maybe()
			mockSack.On("WindowsUpdatesCacheStore").Return(inmemory.NewStore()).Maybe()

			// Set up the startup settings store -- opening RW ensures that the db exists
			// with the appropriate migrations.
			startupSettingsStore, err := agentsqlite.OpenRW(t.Context(), rootDir, agentsqlite.StartupSettingsStore)
			require.NoError(t, err, "initializing startup settings store")
			require.NoError(t, startupSettingsStore.Close())

			// Set up our replacement for stdin
			inFilePath := filepath.Join(rootDir, "in.txt")
			inFile, err := os.Create(inFilePath)
			require.NoError(t, err)
			defer inFile.Close()

			// For our stdin replacement, we want at least some data in the file  -- we want interactive to run a query,
			// and then exit.
			commandContents := `
select * from time;
.exit
`
			err = os.WriteFile(inFilePath, []byte(commandContents), 0755)
			require.NoError(t, err)

			// Set up our replacement for stdout
			outFilePath := filepath.Join(rootDir, "out.txt")
			outFile, err := os.Create(outFilePath)
			require.NoError(t, err)
			defer outFile.Close()

			// Set up our replacement for stderr
			errFilePath := filepath.Join(rootDir, "err.txt")
			errFile, err := os.Create(errFilePath)
			require.NoError(t, err)
			defer errFile.Close()

			// Make sure the process starts in a timely fashion
			var proc *os.Process
			startErr := make(chan error)
			startTime := time.Now()
			go func() {
				proc, _, err = StartProcess(mockSack, rootDir, inFile, outFile, errFile)
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
				errContents, _ := os.ReadFile(errFilePath)
				outContents, _ := os.ReadFile(outFilePath)
				t.Errorf("process did not start before timeout: started at %s, failed at %s: interactive logs:\n%s\nosquery logs:\n%s\nosquery output:\n%s\n", startTime.String(), time.Now().String(), logBytes.String(), string(errContents), string(outContents))
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
					errContents, _ := os.ReadFile(errFilePath)
					t.Error("process did not exit before timeout", fmt.Sprintf("interactive logs:\n%s\nosquery logs:\n%s\n", logBytes.String(), string(errContents)))
					t.FailNow()
				}

				// Confirm we got output, indicating that osquery actually ran a query
				outContents, err := os.ReadFile(outFilePath)
				require.NoError(t, err)
				require.Greater(t, len(outContents), 0)

				// Response data from `time` should include the `timestamp` column and the current year, at least
				require.Contains(t, string(outContents), "timestamp")
				require.Contains(t, string(outContents), strconv.Itoa(time.Now().Year()))
			} else {
				require.Nil(t, proc)
			}
		})
	}
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
