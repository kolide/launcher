package actionmiddleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/control/actionmiddleware/mocks"
	"github.com/kolide/launcher/pkg/agent/storage"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	testUpdaterType    = "test-updater-type"
	anotherUpdaterType = "another-updater-type"
)

func TestUpdate_HandlesDuplicates(t *testing.T) {
	t.Parallel()

	// Queue up two duplicate actions
	testId := ulid.New()
	testActions := []action{
		{
			ID:         testId,
			ValidUntil: getValidUntil(),
			Type:       testUpdaterType,
		},
		{
			ID:         testId,
			ValidUntil: getValidUntil(),
			Type:       testUpdaterType,
		},
	}
	testNotificationsRaw, err := json.Marshal(testActions)
	require.NoError(t, err)

	testNotificationsData := bytes.NewReader(testNotificationsRaw)

	// Expect that the notifier is called only once, to send the first notification
	mockUpdater := mocks.NewUpdater(t)
	mockUpdater.On("Update", mock.Anything).Return(nil).Once()

	actionMiddleWare := New(WithUpdater(testUpdaterType, mockUpdater))

	require.NoError(t, actionMiddleWare.Update(testNotificationsData))
}

func TestUpdate_HandlesMultipleTypes(t *testing.T) {
	t.Parallel()

	testActions := []action{
		{
			ID:         ulid.New(),
			ValidUntil: getValidUntil(),
			Type:       testUpdaterType,
		},
		{
			ID:         ulid.New(),
			ValidUntil: getValidUntil(),
			Type:       anotherUpdaterType,
		},
		{
			ID:         ulid.New(),
			ValidUntil: getValidUntil(),
			Type:       anotherUpdaterType,
		},
		{
			// missing type
			ID:         ulid.New(),
			ValidUntil: getValidUntil(),
		},
		{
			// missing valid until
			ID:   ulid.New(),
			Type: anotherUpdaterType,
		},
		{
			// non existent type
			ID:         ulid.New(),
			Type:       "type-not-found",
			ValidUntil: getValidUntil(),
		},
	}
	testNotificationsRaw, err := json.Marshal(testActions)
	require.NoError(t, err)

	testNotificationsData := bytes.NewReader(testNotificationsRaw)

	// Expect that the notifier is called only once, to send the first notification
	mockUpdater := mocks.NewUpdater(t)
	mockUpdater.On("Update", mock.Anything).Return(nil).Once()

	anotherMockUpdater := mocks.NewUpdater(t)
	anotherMockUpdater.On("Update", mock.Anything).Return(nil).Twice()

	actionMiddleWare := New(
		WithUpdater(testUpdaterType, mockUpdater),
		WithUpdater(anotherUpdaterType, anotherMockUpdater),
	)

	require.NoError(t, actionMiddleWare.Update(testNotificationsData))
}

func TestUpdate_HandlesDuplicatesWhenFirstActionCouldNotBeSent(t *testing.T) {
	t.Parallel()

	// Queue up two duplicate notifications
	testId := ulid.New()
	actions := []action{
		{
			ID:         testId,
			ValidUntil: getValidUntil(),
			Type:       testUpdaterType,
		},
		{
			ID:         testId,
			ValidUntil: getValidUntil(),
			Type:       testUpdaterType,
		},
	}
	testActionsRaw, err := json.Marshal(actions)
	require.NoError(t, err)
	testActionsData := bytes.NewReader(testActionsRaw)

	// Expect that the notifier is called twice: once to unsuccessfully send the first notification, and again to send the duplicate successfully
	mockUpdater := mocks.NewUpdater(t)
	errorCall := mockUpdater.On("Update", mock.Anything).Return(errors.New("test error")).Once()
	mockUpdater.On("Update", mock.Anything).Return(nil).NotBefore(errorCall).Once()

	// Call update and assert our expectations about sent notifications
	actionMiddleWare := New(WithUpdater(testUpdaterType, mockUpdater))
	require.NoError(t, actionMiddleWare.Update(testActionsData))
}

func TestCleanup(t *testing.T) {
	t.Parallel()

	mockUpdater := mocks.NewUpdater(t)
	store := setupStorage(t)
	var logBytes threadsafebuffer.ThreadSafeBuffer
	logger := log.NewLogfmtLogger(&logBytes)
	amw := New(
		WithStore(store),
		WithUpdater(testUpdaterType, mockUpdater),
		WithLogger(logger),
		WithActionRetentionPeriod(1*time.Hour),
		WithCleanupInterval(100*time.Millisecond),
		WithContext(context.Background()),
	)

	// Save two entries in the db -- one sent a year ago, and one sent now.
	notificationIdToDelete := "should_be_deleted"
	amw.storeActionRecord(action{
		ID:          notificationIdToDelete,
		ProcessedAt: time.Now().Add(-365 * 24 * time.Hour),
		Type:        testUpdaterType,
	})
	notificationIdToRetain := "should_be_retained"
	amw.storeActionRecord(action{
		ID:          notificationIdToRetain,
		ProcessedAt: time.Now(),
		Type:        testUpdaterType,
	})

	// Confirm we have both entries in the db.
	oldNotificationRecord, err := store.Get([]byte(notificationIdToDelete))
	require.NotNil(t, oldNotificationRecord, "old notification was not seeded in db")
	require.NoError(t, err)

	newNotificationRecord, err := store.Get([]byte(notificationIdToRetain))
	require.NotNil(t, newNotificationRecord, "new notification was not seeded in db")
	require.NoError(t, err)

	// start clean up
	go func() {
		amw.StartCleanup()
	}()

	// give it a chance to run
	time.Sleep(500 * time.Millisecond)

	// Confirm that the old notification record was deleted, and the new one was not.
	oldNotificationRecord, err = store.Get([]byte(notificationIdToDelete))
	require.Nil(t, oldNotificationRecord, "old notification was not cleaned up but should have been")
	require.NoError(t, err)

	newNotificationRecord, err = store.Get([]byte(notificationIdToRetain))
	require.NotNil(t, newNotificationRecord, "new notification was cleaned up but should not have been")
	require.NoError(t, err)

	// stop
	amw.StopCleanup(nil)
	// give log a chance to log
	time.Sleep(500 * time.Millisecond)
	require.Contains(t, logBytes.String(), "cleanup")
}

func TestUpdate_HandlesMalformedActions(t *testing.T) {
	t.Parallel()

	// Queue up two notifications -- one malformed, one correctly formed
	testId := ulid.New()
	goodAction := action{
		ID:         testId,
		ValidUntil: getValidUntil(),
		Type:       testUpdaterType,
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

	mockUpdater := mocks.NewUpdater(t)

	// Expect that the updater is still called once, to send the good notification
	mockUpdater.On("Update", bytes.NewReader(goodActionRaw)).Return(nil)
	actionMiddleWare := New(WithUpdater(testUpdaterType, mockUpdater))
	require.NoError(t, actionMiddleWare.Update(testActionsData))
}

func setupStorage(t *testing.T) types.KVStore {
	s, err := storageci.NewStore(t, log.NewNopLogger(), storage.SentNotificationsStore.String())
	require.NoError(t, err)
	return s
}

func getValidUntil() int64 {
	return time.Now().Add(1 * time.Hour).Unix()
}
