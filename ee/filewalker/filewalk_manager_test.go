package filewalker

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

func TestExecute(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	mockKnapsack := typesmocks.NewKnapsack(t)
	cfgStore, err := storageci.NewStore(t, slogger, storage.FilewalkConfigStore.String())
	require.NoError(t, err)
	mockKnapsack.On("FilewalkConfigStore").Return(cfgStore)
	resultsStore, err := storageci.NewStore(t, slogger, storage.FilewalkResultsStore.String())
	require.NoError(t, err)
	mockKnapsack.On("FilewalkResultsStore").Return(resultsStore)

	// Set up our filewalk config in the store
	cfg := generateCfgWithSeeding(t, 500*time.Millisecond, 2, nil, 1)
	cfgRaw, err := json.Marshal(cfg)
	require.NoError(t, err)
	testTableName := "TestExecute_tbl"
	cfgStore.Set([]byte(testTableName), cfgRaw)

	// Init filewalk manager
	filewalkManager := New(mockKnapsack, slogger)

	// Run the manager and let it spin up filewalkers
	go filewalkManager.Execute()
	time.Sleep(3 * cfg.WalkInterval)

	// Confirm we have one filewalker
	filewalkManager.filewalkersLock.Lock()
	require.Equal(t, 1, len(filewalkManager.filewalkers))
	require.Contains(t, filewalkManager.filewalkers, testTableName)
	filewalkManager.filewalkersLock.Unlock()

	// Confirm we have results
	rawResults, err := resultsStore.Get([]byte(testTableName))
	require.NoError(t, err)
	results := make([]string, 0)
	require.NoError(t, json.Unmarshal(rawResults, &results))
	require.Equal(t, 2, len(results)) // 2 directories, 1 file per directory
}

func TestPing(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	mockKnapsack := typesmocks.NewKnapsack(t)
	cfgStore, err := storageci.NewStore(t, slogger, storage.FilewalkConfigStore.String())
	require.NoError(t, err)
	mockKnapsack.On("FilewalkConfigStore").Return(cfgStore)
	resultsStore, err := storageci.NewStore(t, slogger, storage.FilewalkResultsStore.String())
	require.NoError(t, err)
	mockKnapsack.On("FilewalkResultsStore").Return(resultsStore)

	// Set up our filewalk config in the store
	cfg := generateCfgWithSeeding(t, 500*time.Millisecond, 1, nil, 3)
	cfgRaw, err := json.Marshal(cfg)
	require.NoError(t, err)
	firstTestTableName := "TestPing_tbl"
	cfgStore.Set([]byte(firstTestTableName), cfgRaw)

	// Init filewalk manager
	filewalkManager := New(mockKnapsack, slogger)

	// Run the manager and let it spin up filewalkers
	go filewalkManager.Execute()
	time.Sleep(3 * cfg.WalkInterval)

	// Confirm we have one filewalker
	filewalkManager.filewalkersLock.Lock()
	require.Equal(t, 1, len(filewalkManager.filewalkers))
	require.Contains(t, filewalkManager.filewalkers, firstTestTableName)
	filewalkManager.filewalkersLock.Unlock()

	// Confirm we have results
	rawResults, err := resultsStore.Get([]byte(firstTestTableName))
	require.NoError(t, err)
	results := make([]string, 0)
	require.NoError(t, json.Unmarshal(rawResults, &results))
	require.Equal(t, 3, len(results)) // 1 directory, 3 files per directory

	// Prepare an update: update the config for the existing filewalker
	testRegexp := regexp.MustCompile(`.*\.doc`)
	newCfg := generateCfgWithSeeding(t, 500*time.Millisecond, 2, testRegexp, 3)
	newCfgRaw, err := json.Marshal(newCfg)
	require.NoError(t, err)
	cfgStore.Set([]byte(firstTestTableName), newCfgRaw)

	// Call Ping
	filewalkManager.Ping()
	time.Sleep(3 * cfg.WalkInterval)

	// Confirm we still have one filewalker
	filewalkManager.filewalkersLock.Lock()
	require.Equal(t, 1, len(filewalkManager.filewalkers))
	require.Contains(t, filewalkManager.filewalkers, firstTestTableName)
	filewalkManager.filewalkersLock.Unlock()

	// Confirm we have results for that filewalker
	updatedRawResults, err := resultsStore.Get([]byte(firstTestTableName))
	require.NoError(t, err)
	updatedResults := make([]string, 0)
	require.NoError(t, json.Unmarshal(updatedRawResults, &updatedResults))
	require.Equal(t, 6, len(updatedResults)) // 2 directories, 3 files per directory

	// Prepare an update: add a new filewalker
	secondFilewalkerCfg := generateCfgWithSeeding(t, 500*time.Millisecond, 2, nil, 2)
	secondCfgRaw, err := json.Marshal(secondFilewalkerCfg)
	require.NoError(t, err)
	secondTestTableName := "TestPing2_tbl"
	cfgStore.Set([]byte(secondTestTableName), secondCfgRaw)

	// Call Ping
	filewalkManager.Ping()
	time.Sleep(3 * cfg.WalkInterval)

	// Confirm we now have two filewalkers
	filewalkManager.filewalkersLock.Lock()
	require.Equal(t, 2, len(filewalkManager.filewalkers))
	require.Contains(t, filewalkManager.filewalkers, firstTestTableName)
	require.Contains(t, filewalkManager.filewalkers, secondTestTableName)
	filewalkManager.filewalkersLock.Unlock()

	// Confirm we have results for the new filewalker
	secondTableRawResults, err := resultsStore.Get([]byte(secondTestTableName))
	require.NoError(t, err)
	secondTableResults := make([]string, 0)
	require.NoError(t, json.Unmarshal(secondTableRawResults, &secondTableResults))
	require.Equal(t, 4, len(secondTableResults)) // 2 directories, 2 files per directory

	// Prepare an update: delete the new filewalker
	require.NoError(t, cfgStore.Delete([]byte(secondTestTableName)))

	// Call Ping
	filewalkManager.Ping()
	time.Sleep(3 * cfg.WalkInterval)

	// Confirm we're back to one filewalker
	filewalkManager.filewalkersLock.Lock()
	require.Equal(t, 1, len(filewalkManager.filewalkers))
	require.Contains(t, filewalkManager.filewalkers, firstTestTableName)
	require.NotContains(t, filewalkManager.filewalkers, secondTestTableName)
	filewalkManager.filewalkersLock.Unlock()
}

func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	mockKnapsack := typesmocks.NewKnapsack(t)
	cfgStore, err := storageci.NewStore(t, slogger, storage.FilewalkConfigStore.String())
	require.NoError(t, err)
	mockKnapsack.On("FilewalkConfigStore").Return(cfgStore)

	// Init filewalk manager
	filewalkManager := New(mockKnapsack, slogger)

	// Let the filewalk manager run for a bit
	go filewalkManager.Execute()
	time.Sleep(3 * time.Second)
	interruptStart := time.Now()
	filewalkManager.Interrupt(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			filewalkManager.Interrupt(nil)
			interruptComplete <- struct{}{}
		}()
	}

	receivedInterrupts := 0
	for receivedInterrupts < expectedInterrupts {
		select {
		case <-interruptComplete:
			receivedInterrupts += 1
			continue
		case <-time.After(5 * time.Second):
			t.Errorf("could not call interrupt multiple times and return within 5 seconds -- interrupted at %s, received %d interrupts before timeout; logs: \n%s\n", interruptStart.String(), receivedInterrupts, logBytes.String())
			t.FailNow()
		}
	}

	require.Equal(t, expectedInterrupts, receivedInterrupts)
}

// generateCfgWithSeeding creates a filewalkConfig with the given parameters, and creates temporary directories
// with files in them for a filewalker to read. The filenameRegex must be simple -- construct it with a single
// `.*` to be replaced with a random string. The function will fail if it is unable to generate a matching filename.
func generateCfgWithSeeding(t *testing.T, walkInterval time.Duration, numDirs int, filenameRegex *regexp.Regexp, numFilesPerDir int) filewalkConfig {
	rootDirs := make([]string, numDirs)
	skipDirs := make([]*regexp.Regexp, numDirs)

	for i := range rootDirs {
		currentRootDir := t.TempDir()
		rootDirs[i] = currentRootDir

		for j := range numFilesPerDir {
			newFilename := fmt.Sprintf("temp-%d.txt", j)
			if filenameRegex != nil {
				// Try to generate a filename that will match
				newFilename = strings.ReplaceAll(filenameRegex.String(), `\`, "")     // Remove escape characters
				newFilename = strings.ReplaceAll(newFilename, ".*", uuid.NewString()) // Replace wildcard
				require.True(t, filenameRegex.MatchString(newFilename), "could not generate matching filename")
			}

			expectedFile := filepath.Join(currentRootDir, newFilename)
			require.NoError(t, os.WriteFile(expectedFile, []byte("test"), 0755))
		}

		skipDir := filepath.Join(currentRootDir, uuid.NewString())
		require.NoError(t, os.Mkdir(skipDir, 0755))
		skipDirRegexStr := strings.ReplaceAll(skipDir, `\`, `\\`) // Escape for Windows
		skipDirRegexStr = strings.ReplaceAll(skipDirRegexStr, `/`, `\/`)
		skipDirRegexp := regexp.MustCompile(skipDirRegexStr)
		require.True(t, skipDirRegexp.MatchString(skipDir))
		skipFileName := "skipme.txt"
		if filenameRegex != nil {
			// Try to generate a filename that will match (so we can confirm the directory is skipped due to skipDirs and not to a regex mismatch)
			skipFileName = strings.ReplaceAll(filenameRegex.String(), `\`, "")      // Remove escape characters
			skipFileName = strings.ReplaceAll(skipFileName, ".*", uuid.NewString()) // Replace wildcard
			require.True(t, filenameRegex.MatchString(skipFileName), "could not generate matching filename")
		}
		skipFile := filepath.Join(skipDir, skipFileName)
		require.NoError(t, os.WriteFile(skipFile, []byte("test"), 0755))
		skipDirs[i] = skipDirRegexp
	}

	return filewalkConfig{
		WalkInterval: walkInterval,
		filewalkDefinition: filewalkDefinition{
			RootDirs:      &rootDirs,
			FileNameRegex: filenameRegex,
			SkipDirs:      &skipDirs,
		},
	}
}
