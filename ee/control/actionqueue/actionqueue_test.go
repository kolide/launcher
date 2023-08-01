package actionqueue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/control/actionqueue/mocks"
	"github.com/kolide/launcher/pkg/agent/storage"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	testActorType        = "test-actor-type"
	anotherTestActorType = "another-actor-type"
)

func TestDoAction_HandlesDuplicates(t *testing.T) {
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
	mockActor.On("DoAction", mock.Anything).Return(nil).Once()

	actionqueue := New()
	actionqueue.RegisterActor(testActorType, mockActor)

	require.NoError(t, actionqueue.Update(testActionsData))
}

func TestDoAction_HandlesMultipleTypes(t *testing.T) {
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
	mockActor.On("DoAction", mock.Anything).Return(nil).Once()

	anotherMockActor := mocks.NewActor(t)
	anotherMockActor.On("DoAction", mock.Anything).Return(nil).Twice()

	actionqueue := New()

	actionqueue.RegisterActor(testActorType, mockActor)
	actionqueue.RegisterActor(anotherTestActorType, anotherMockActor)

	require.NoError(t, actionqueue.Update(testActionsData))
}

func TestDoAction_HandlesDuplicatesWhenFirstActionCouldNotBeSent(t *testing.T) {
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
	testActionsData := bytes.NewReader(testActionsRaw)

	// Expect that the actor is called twice: once to unsuccessfully send the first action, and again to send the duplicate successfully
	mockActor := mocks.NewActor(t)
	errorCall := mockActor.On("DoAction", mock.Anything).Return(errors.New("test error")).Once()
	mockActor.On("DoAction", mock.Anything).Return(nil).NotBefore(errorCall).Once()

	// Call DoAction and assert our expectations about completed actions
	actionqueue := New()
	actionqueue.RegisterActor(testActorType, mockActor)
	require.NoError(t, actionqueue.Update(testActionsData))
}

func TestCleanup(t *testing.T) {
	t.Parallel()

	mockActor := mocks.NewActor(t)
	store := setupStorage(t)
	var logBytes threadsafebuffer.ThreadSafeBuffer
	logger := log.NewLogfmtLogger(&logBytes)
	actionQueue := New(
		WithStore(store),
		WithLogger(logger),
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

func TestDoAction_HandlesMalformedActions(t *testing.T) {
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

	// Expect that the DoActionr is still called once, to send do the good action
	mockActioner.On("DoAction", bytes.NewReader(goodActionRaw)).Return(nil)
	actionqueue := New()
	actionqueue.RegisterActor(testActorType, mockActioner)
	require.NoError(t, actionqueue.Update(testActionsData))
}

func setupStorage(t *testing.T) types.KVStore {
	s, err := storageci.NewStore(t, log.NewNopLogger(), storage.ControlServerActionsStore.String())
	require.NoError(t, err)
	return s
}

func getValidUntil() int64 {
	return time.Now().Add(1 * time.Hour).Unix()
}
