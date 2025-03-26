package actionqueue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/types"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/control/actionqueue/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	testActorType        = "test-actor-type"
	anotherTestActorType = "another-actor-type"
)

func TestActionQueue_HandlesDuplicates(t *testing.T) {
	t.Parallel()

	// Queue up two duplicate actions
	testId := ulid.New()
	testActions := []action{
		{
			ID:         testId,
			ValidUntil: getValidUntil(),
			Type:       testActorType,
		},
		{
			ID:         testId,
			ValidUntil: getValidUntil(),
			Type:       testActorType,
		},
	}
	testActionsRaw, err := json.Marshal(testActions)
	require.NoError(t, err)

	testActionsData := bytes.NewReader(testActionsRaw)

	// Expect that the actor is called only once, to send the first action
	mockActor := mocks.NewActor(t)
	mockActor.On("Do", mock.Anything).Return(nil).Once()

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())

	actionqueue := New(mockKnapsack)
	actionqueue.RegisterActor(testActorType, mockActor)

	require.NoError(t, actionqueue.Update(testActionsData))
}

func TestActionQueue_ChecksOldNotificationStore(t *testing.T) {
	t.Parallel()

	oldNotification := action{
		ID:         ulid.New(),
		ValidUntil: getValidUntil(),
		Type:       testActorType,
	}

	newAction := action{
		ID:         ulid.New(),
		ValidUntil: getValidUntil(),
		Type:       testActorType,
	}

	testActions := []action{
		oldNotification,
		newAction,
	}

	testActionsRaw, err := json.Marshal(testActions)
	require.NoError(t, err)

	testActionsData := bytes.NewReader(testActionsRaw)

	oldNotificationStore := setupStorage(t)
	oldNotificationStore.Set([]byte(oldNotification.ID), mustJsonMarshal(t, oldNotification))

	// Expect that the actor is only called once, to send the new action
	mockActor := mocks.NewActor(t)
	mockActor.On("Do", mock.Anything).Return(nil).Once()

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())

	actionqueue := New(mockKnapsack, WithOldNotificationsStore(oldNotificationStore))

	actionqueue.RegisterActor(testActorType, mockActor)

	require.NoError(t, actionqueue.Update(testActionsData))
}

func TestActionQueue_HandlesMultipleActorTypes(t *testing.T) {
	t.Parallel()

	testActions := []action{
		{
			ID:         ulid.New(),
			ValidUntil: getValidUntil(),
			Type:       testActorType,
		},
		{
			ID:         ulid.New(),
			ValidUntil: getValidUntil(),
			Type:       anotherTestActorType,
		},
		{
			ID:         ulid.New(),
			ValidUntil: getValidUntil(),
			Type:       anotherTestActorType,
		},
		{
			// missing type
			ID:         ulid.New(),
			ValidUntil: getValidUntil(),
		},
		{
			// missing valid until
			ID:   ulid.New(),
			Type: anotherTestActorType,
		},
		{
			// non existent type
			ID:         ulid.New(),
			Type:       "type-not-found",
			ValidUntil: getValidUntil(),
		},
	}
	testActionsRaw, err := json.Marshal(testActions)
	require.NoError(t, err)

	testActionsData := bytes.NewReader(testActionsRaw)

	// Expect that the actor is called only once, to send the first action
	mockActor := mocks.NewActor(t)
	mockActor.On("Do", mock.Anything).Return(nil).Once()

	anotherMockActor := mocks.NewActor(t)
	anotherMockActor.On("Do", mock.Anything).Return(nil).Twice()

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())

	actionqueue := New(mockKnapsack)

	actionqueue.RegisterActor(testActorType, mockActor)
	actionqueue.RegisterActor(anotherTestActorType, anotherMockActor)

	require.NoError(t, actionqueue.Update(testActionsData))
}

func TestActionQueue_HandlesDuplicatesWhenFirstActionCouldNotBeSent(t *testing.T) {
	t.Parallel()

	// Queue up two duplicate actions
	testId := ulid.New()
	actions := []action{
		{
			ID:         testId,
			ValidUntil: getValidUntil(),
			Type:       testActorType,
		},
		{
			ID:         testId,
			ValidUntil: getValidUntil(),
			Type:       testActorType,
		},
	}
	testActionsRaw, err := json.Marshal(actions)
	require.NoError(t, err)

	// Expect that the actor is called twice: once to unsuccessfully send the first action, and again to send the duplicate successfully
	mockActor := mocks.NewActor(t)
	errorCall := mockActor.On("Do", mock.Anything).Return(errors.New("test error")).Once()
	mockActor.On("Do", mock.Anything).Return(nil).NotBefore(errorCall).Once()

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())

	// Call Do and assert our expectations about completed actions
	actionqueue := New(mockKnapsack)
	actionqueue.RegisterActor(testActorType, mockActor)
	// First attempt fails
	err = actionqueue.Update(bytes.NewReader(testActionsRaw))
	require.Error(t, err)

	// Second attempt succeeds
	err = actionqueue.Update(bytes.NewReader(testActionsRaw))
	require.NoError(t, err)
}

func TestCleanup(t *testing.T) {
	t.Parallel()

	mockActor := mocks.NewActor(t)
	store := setupStorage(t)
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := multislogger.New(slog.NewJSONHandler(&logBytes, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("Slogger").Return(slogger.Logger)
	actionQueue := New(
		mockKnapsack,
		WithStore(store),
		WithCleanupInterval(100*time.Millisecond),
		WithContext(context.Background()),
	)
	actionQueue.RegisterActor(testActorType, mockActor)

	// Save two entries in the db -- one sent a year ago, and one sent now.
	actionsToDelete := "should_be_deleted"
	actionQueue.storeActionRecord(action{
		ID:          actionsToDelete,
		ProcessedAt: time.Now().Add(-365 * 24 * time.Hour),
		Type:        testActorType,
	})
	actionsToReturn := "should_be_retained"
	actionQueue.storeActionRecord(action{
		ID:          actionsToReturn,
		ProcessedAt: time.Now(),
		Type:        testActorType,
	})

	// Confirm we have both entries in the db.
	oldActionRecord, err := store.Get([]byte(actionsToDelete))
	require.NotNil(t, oldActionRecord, "old action was not seeded in db")
	require.NoError(t, err)

	newActionRecord, err := store.Get([]byte(actionsToReturn))
	require.NotNil(t, newActionRecord, "new action was not seeded in db")
	require.NoError(t, err)

	// start clean up
	go func() {
		actionQueue.StartCleanup()
	}()

	// give it a chance to run
	time.Sleep(500 * time.Millisecond)

	// Confirm that the old action record was deleted, and the new one was not.
	oldActionRecord, err = store.Get([]byte(actionsToDelete))
	require.Nil(t, oldActionRecord, "old action was not cleaned up but should have been")
	require.NoError(t, err)

	newActionRecord, err = store.Get([]byte(actionsToReturn))
	require.NotNil(t, newActionRecord, "new action was cleaned up but should not have been")
	require.NoError(t, err)

	// stop
	actionQueue.StopCleanup(nil)
	// give log a chance to log
	time.Sleep(500 * time.Millisecond)
	require.Contains(t, logBytes.String(), "cleanup")
}

func TestStopCleanup_Multiple(t *testing.T) {
	t.Parallel()

	mockActor := mocks.NewActor(t)
	store := setupStorage(t)
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
	actionQueue := New(
		mockKnapsack,
		WithStore(store),
		WithCleanupInterval(100*time.Millisecond),
		WithContext(context.Background()),
	)
	actionQueue.RegisterActor(testActorType, mockActor)

	// start clean up
	go actionQueue.StartCleanup()
	time.Sleep(3 * time.Second)
	actionQueue.StopCleanup(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			actionQueue.StopCleanup(nil)
			interruptComplete <- struct{}{}
		}()
	}

	receivedInterrupts := 0
	for {
		if receivedInterrupts >= expectedInterrupts {
			break
		}

		select {
		case <-interruptComplete:
			receivedInterrupts += 1
			continue
		case <-time.After(5 * time.Second):
			t.Errorf("could not call interrupt multiple times and return within 5 seconds -- received %d interrupts before timeout", receivedInterrupts)
			t.FailNow()
		}
	}

	require.Equal(t, expectedInterrupts, receivedInterrupts)
}

func TestActionQueue_HandlesMalformedActions(t *testing.T) {
	t.Parallel()

	// Queue up two actions -- one malformed, one correctly formed
	testId := ulid.New()
	goodAction := action{
		ID:         testId,
		ValidUntil: getValidUntil(),
		Type:       testActorType,
	}
	goodActionRaw, err := json.Marshal(goodAction)
	require.NoError(t, err)

	badAction := struct {
		AnUnknownField      string `json:"an_unknown_field"`
		AnotherUnknownField bool   `json:"another_unknown_field"`
	}{
		AnUnknownField:      testId,
		AnotherUnknownField: true,
	}
	badActionRaw, err := json.Marshal(badAction)
	require.NoError(t, err)

	testActions := []json.RawMessage{goodActionRaw, badActionRaw}
	testActionsRaw, err := json.Marshal(testActions)
	require.NoError(t, err)

	testActionsData := bytes.NewReader(testActionsRaw)

	mockActioner := mocks.NewActor(t)

	// Expect that the Do is still called once, to send do the good action
	mockActioner.On("Do", bytes.NewReader(goodActionRaw)).Return(nil)

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())

	actionqueue := New(mockKnapsack)
	actionqueue.RegisterActor(testActorType, mockActioner)
	require.NoError(t, actionqueue.Update(testActionsData))
}

func setupStorage(t *testing.T) types.KVStore {
	s, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ControlServerActionsStore.String())
	require.NoError(t, err)
	return s
}

func getValidUntil() int64 {
	return time.Now().Add(1 * time.Hour).Unix()
}

func mustJsonMarshal(t *testing.T, v interface{}) []byte {
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
