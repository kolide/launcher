package remoterestartconsumer

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/kolide/kit/ulid"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func TestDo(t *testing.T) {
	t.Parallel()

	currentRunId := ulid.New()

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
	mockKnapsack.On("GetRunID").Return(currentRunId)

	remoteRestarter := New(mockKnapsack)

	testAction := remoteRestartAction{
		RunID: currentRunId,
	}
	testActionRaw, err := json.Marshal(testAction)
	require.NoError(t, err)

	// We don't expect an error because we should process the action
	require.NoError(t, remoteRestarter.Do(bytes.NewReader(testActionRaw)), "expected no error processing valid remote restart action")

	// The restarter should delay before sending an error to `signalRestart`
	require.Len(t, remoteRestarter.signalRestart, 0, "expected restarter to delay before signal for restart but channel is already has item in it")
	time.Sleep(restartDelay + 2*time.Second)
	require.Len(t, remoteRestarter.signalRestart, 1, "expected restarter to signal for restart but channel is empty after delay")
}

func TestDo_DoesNotSignalRestartWhenRunIDDoesNotMatch(t *testing.T) {
	t.Parallel()

	currentRunId := ulid.New()

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
	mockKnapsack.On("GetRunID").Return(currentRunId)

	remoteRestarter := New(mockKnapsack)

	testAction := remoteRestartAction{
		RunID: ulid.New(), // run ID will not match `currentRunId`
	}
	testActionRaw, err := json.Marshal(testAction)
	require.NoError(t, err)

	// We don't expect an error because we want to discard this action
	require.NoError(t, remoteRestarter.Do(bytes.NewReader(testActionRaw)), "should not return error for old run ID")

	// The restarter should not send an error to `signalRestart`
	time.Sleep(restartDelay + 2*time.Second)
	require.Len(t, remoteRestarter.signalRestart, 0, "restarter should not have signaled for a restart, but channel is not empty")
}

func TestDo_DoesNotSignalRestartWhenRunIDIsEmpty(t *testing.T) {
	t.Parallel()

	currentRunId := ulid.New()

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
	mockKnapsack.On("GetRunID").Return(currentRunId)

	remoteRestarter := New(mockKnapsack)

	testAction := remoteRestartAction{
		RunID: "", // run ID will not match `currentRunId`
	}
	testActionRaw, err := json.Marshal(testAction)
	require.NoError(t, err)

	// We don't expect an error because we want to discard this action
	require.NoError(t, remoteRestarter.Do(bytes.NewReader(testActionRaw)), "should not return error for empty run ID")

	// The restarter should not send an error to `signalRestart`
	time.Sleep(restartDelay + 2*time.Second)
	require.Len(t, remoteRestarter.signalRestart, 0, "restarter should not have signaled for a restart, but channel is not empty")
}
