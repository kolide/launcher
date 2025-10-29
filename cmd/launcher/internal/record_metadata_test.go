package internal

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

func TestPing(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName            string
		startingMetadata        *metadata
		newMetadata             *metadata
		shouldLogMetadataChange bool
	}{
		{
			testCaseName:     "setting metadata for the first time",
			startingMetadata: nil,
			newMetadata: &metadata{
				DeviceId:           "2",
				OrganizationId:     "200",
				OrganizationMunemo: "new-test",
			},
			shouldLogMetadataChange: false,
		},
		{
			testCaseName: "some metadata changed",
			startingMetadata: &metadata{
				DeviceId:           "1",
				OrganizationId:     "100",
				OrganizationMunemo: "old-test",
			},
			newMetadata: &metadata{
				DeviceId:           "2",
				OrganizationId:     "100",
				OrganizationMunemo: "old-test",
			},
			shouldLogMetadataChange: true,
		},
		{
			testCaseName: "all metadata changed",
			startingMetadata: &metadata{
				DeviceId:           "1",
				OrganizationId:     "100",
				OrganizationMunemo: "old-test",
			},
			newMetadata: &metadata{
				DeviceId:           "2",
				OrganizationId:     "200",
				OrganizationMunemo: "new-test",
			},
			shouldLogMetadataChange: true,
		},
		{
			testCaseName: "no metadata changed",
			startingMetadata: &metadata{
				DeviceId:           "1",
				OrganizationId:     "100",
				OrganizationMunemo: "old-test",
			},
			newMetadata: &metadata{
				DeviceId:           "1",
				OrganizationId:     "100",
				OrganizationMunemo: "old-test",
			},
			shouldLogMetadataChange: false,
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			// Set up slogger with buffer -- we want to confirm that we log any important changes in metadata
			var logBytes threadsafebuffer.ThreadSafeBuffer
			slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			}))

			// Set up knapsack dependencies
			rootDir := t.TempDir()
			mockKnapsack := typesmocks.NewKnapsack(t)
			mockKnapsack.On("RootDirectory").Return(rootDir)
			testServerProvidedDataStore, err := storageci.NewStore(t, slogger, storage.ServerProvidedDataStore.String())
			require.NoError(t, err, "could not create test server provided data store")
			mockKnapsack.On("ServerProvidedDataStore").Return(testServerProvidedDataStore).Maybe()

			// Set up metadata writer
			testMetadataWriter := NewMetadataWriter(slogger, mockKnapsack)

			// Confirm blank slate state
			metadataFile := filepath.Join(rootDir, "metadata.json")
			_, err = os.Stat(metadataFile)
			require.True(t, errors.Is(err, os.ErrNotExist), "metadata file should not exist yet")

			// Set up existing metadata file, if required
			if tt.startingMetadata != nil {
				// Set starting data in store
				testServerProvidedDataStore.Set([]byte("device_id"), []byte(tt.startingMetadata.DeviceId))
				testServerProvidedDataStore.Set([]byte("organization_id"), []byte(tt.startingMetadata.OrganizationId))
				testServerProvidedDataStore.Set([]byte("munemo"), []byte(tt.startingMetadata.OrganizationMunemo))

				// Prompt metadata writer to write data to file
				testMetadataWriter.Ping()

				// Confirm we set the starting data appropriately
				setStartingMetadata := testMetadataWriter.currentRecordedMetadata()
				require.NotNil(t, setStartingMetadata, "metadata not set in file")
				require.Equal(t, tt.startingMetadata.DeviceId, setStartingMetadata.DeviceId)
				require.Equal(t, tt.startingMetadata.OrganizationId, setStartingMetadata.OrganizationId)
				require.Equal(t, tt.startingMetadata.OrganizationMunemo, setStartingMetadata.OrganizationMunemo)

				// Confirm we did not log a change in metadata when setting metadata for the first time
				require.NotContains(t, logBytes.String(), "server metadata changed", "should not have logged server metadata change when initially setting it")
			}

			// Set the updated metadata in the store
			testServerProvidedDataStore.Set([]byte("device_id"), []byte(tt.newMetadata.DeviceId))
			testServerProvidedDataStore.Set([]byte("organization_id"), []byte(tt.newMetadata.OrganizationId))
			testServerProvidedDataStore.Set([]byte("munemo"), []byte(tt.newMetadata.OrganizationMunemo))

			// Test metadata update
			testMetadataWriter.Ping()

			// Confirm we set the new data appropriately
			setNewMetadata := testMetadataWriter.currentRecordedMetadata()
			require.NotNil(t, setNewMetadata, "metadata not set in file")
			require.Equal(t, tt.newMetadata.DeviceId, setNewMetadata.DeviceId)
			require.Equal(t, tt.newMetadata.OrganizationId, setNewMetadata.OrganizationId)
			require.Equal(t, tt.newMetadata.OrganizationMunemo, setNewMetadata.OrganizationMunemo)

			// Make sure we logged the data change, if required
			if tt.shouldLogMetadataChange {
				require.Contains(t, logBytes.String(), "server metadata changed", "should have logged server metadata change")
			} else {
				require.NotContains(t, logBytes.String(), "server metadata changed", "should not have logged server metadata change")
			}
		})
	}
}
