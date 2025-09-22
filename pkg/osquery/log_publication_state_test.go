//nolint:paralleltest
package osquery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kolide/kit/ulid"
	settingsstoremock "github.com/kolide/launcher/pkg/osquery/mocks"
	"github.com/kolide/launcher/pkg/service/mock"
	"github.com/osquery/osquery-go/plugin/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtensionLogPublicationHappyPath(t *testing.T) {
	startingBatchLimitBytes := minBytesPerBatch * 4
	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
			return "", "", false, nil
		},
	}
	k := makeKnapsack(t)
	e, err := NewExtension(context.TODO(), m, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{
		MaxBytesPerBatch: startingBatchLimitBytes,
	})
	require.Nil(t, err)

	// issue a few successful calls, expect that the batch limit is unchanged from the original opts
	for i := 0; i < 3; i++ {
		e.logPublicationState.BeginBatch(time.Now(), true)
		err = e.writeLogsWithReenroll(context.Background(), logger.LogTypeSnapshot, []string{"foobar"}, true)
		assert.Nil(t, err)
		assert.Equal(t, e.logPublicationState.currentMaxBytesPerBatch, startingBatchLimitBytes)
		// always expect that these values are reset between runs
		assert.Equal(t, time.Time{}, e.logPublicationState.currentBatchStartTime)
		assert.False(t, e.logPublicationState.currentBatchBufferFilled)
	}
}

func TestExtensionLogPublicationRespondsToNetworkTimeouts(t *testing.T) {
	numberOfPublicationRounds := 3
	publicationCalledCount := -1
	// startingBatchLimitBytes is set this way to ensure sufficient room for correction in both directions
	startingBatchLimitBytes := minBytesPerBatch * (numberOfPublicationRounds + 1)
	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
			publicationCalledCount++
			switch {
			case publicationCalledCount < numberOfPublicationRounds:
				return "", "", false, errors.New("transport")
			default:
				return "", "", false, nil
			}
		},
	}
	k := makeKnapsack(t)
	e, err := NewExtension(context.TODO(), m, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{
		MaxBytesPerBatch: startingBatchLimitBytes,
	})
	require.Nil(t, err)

	// expect each subsequent failed call to reduce the batch size until the min threshold is reached
	expectedMaxValue := e.Opts.MaxBytesPerBatch
	for i := 0; i < numberOfPublicationRounds; i++ {
		// set the batch state to have started earlier than the 20 seconds threshold ago
		e.logPublicationState.BeginBatch(time.Now().Add(-21*time.Second), true)
		err = e.writeLogsWithReenroll(context.Background(), logger.LogTypeSnapshot, []string{"foobar"}, true)
		assert.NotNil(t, err)
		assert.Less(t, e.logPublicationState.currentMaxBytesPerBatch, expectedMaxValue)
		// always expect that these values are reset between runs
		assert.Equal(t, time.Time{}, e.logPublicationState.currentBatchStartTime)
		assert.False(t, e.logPublicationState.currentBatchBufferFilled)
		expectedMaxValue = e.logPublicationState.currentMaxBytesPerBatch
	}

	// now run a successful publication loop without filling the buffer - we expect
	// this should have no effect on the current batch size
	err = e.writeLogsWithReenroll(context.Background(), logger.LogTypeSnapshot, []string{"foobar"}, true)
	assert.Nil(t, err)
	assert.Equal(t, expectedMaxValue, e.logPublicationState.currentMaxBytesPerBatch)

	// this time mark the buffer as filled for subsequent successful calls and expect that we move back up towards the original batch limit
	for i := 0; i < numberOfPublicationRounds; i++ {
		e.logPublicationState.BeginBatch(time.Now(), true)
		err = e.writeLogsWithReenroll(context.Background(), logger.LogTypeSnapshot, []string{"foobar"}, true)
		assert.Nil(t, err)
		assert.Greater(t, e.logPublicationState.currentMaxBytesPerBatch, expectedMaxValue)
		// always expect that these values are reset between runs
		assert.Equal(t, time.Time{}, e.logPublicationState.currentBatchStartTime)
		assert.False(t, e.logPublicationState.currentBatchBufferFilled)
		expectedMaxValue = e.logPublicationState.currentMaxBytesPerBatch
	}

	// lastly expect that we've returned to our baseline state
	assert.Equal(t, e.logPublicationState.currentMaxBytesPerBatch, startingBatchLimitBytes)
}

func TestExtensionLogPublicationIgnoresNonTimeoutErrors(t *testing.T) {
	startingBatchLimitBytes := minBytesPerBatch * 4
	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
			return "", "", false, errors.New("transport")
		},
	}
	k := makeKnapsack(t)
	e, err := NewExtension(context.TODO(), m, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{
		MaxBytesPerBatch: startingBatchLimitBytes,
	})
	require.Nil(t, err)

	// issue a few calls that error immediately, expect that the batch limit is unchanged from the original opts
	for i := 0; i < 3; i++ {
		e.logPublicationState.BeginBatch(time.Now(), true)
		err = e.writeLogsWithReenroll(context.Background(), logger.LogTypeSnapshot, []string{"foobar"}, true)
		// we still expect an error, but the batch limitation should not have changed
		assert.NotNil(t, err)
		assert.Equal(t, e.logPublicationState.currentMaxBytesPerBatch, startingBatchLimitBytes)
		// always expect that these values are reset between runs
		assert.Equal(t, time.Time{}, e.logPublicationState.currentBatchStartTime)
		assert.False(t, e.logPublicationState.currentBatchBufferFilled)
	}
}
