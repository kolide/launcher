package notificationconsumer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/desktop/user/notify"
	"github.com/kolide/launcher/pkg/agent/storage"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type notifierMock struct{ mock.Mock }

func newNotifierMock() *notifierMock { return &notifierMock{} }

func (nm *notifierMock) SendNotification(n notify.Notification) error {
	args := nm.Called(n)
	return args.Error(0)
}

func TestUpdate_HappyPath(t *testing.T) {
	t.Parallel()

	store := setupStorage(t)
	mockNotifier := newNotifierMock()
	testNc := &NotificationConsumer{
		store:                       store,
		runner:                      mockNotifier,
		logger:                      log.NewNopLogger(),
		notificationRetentionPeriod: defaultRetentionPeriod,
	}

	// Send one notification that we haven't seen before
	testId := ulid.New()
	testTitle := fmt.Sprintf("Test title @ %d - %s", time.Now().UnixMicro(), testId)
	testBody := fmt.Sprintf("Test body @ %d - %s", time.Now().UnixMicro(), testId)
	testActionUri := "https://www.kolide.com"
	testNotifications := []notify.Notification{
		{
			Title:      testTitle,
			Body:       testBody,
			ID:         testId,
			ValidUntil: getValidUntil(),
			ActionUri:  testActionUri,
		},
	}
	testNotificationsRaw, err := json.Marshal(testNotifications)
	require.NoError(t, err)
	testNotificationsData := bytes.NewReader(testNotificationsRaw)

	// Expect that the notifier is called once to send the one notification successfully
	mockNotifier.On("SendNotification", mock.Anything).Return(nil)

	// Call update and assert our expectations about sent notifications
	err = testNc.Update(testNotificationsData)
	require.NoError(t, err)
	mockNotifier.AssertNumberOfCalls(t, "SendNotification", 1)
}

func TestUpdate_HappyPath_NoAction(t *testing.T) {
	t.Parallel()

	store := setupStorage(t)
	mockNotifier := newNotifierMock()
	testNc := &NotificationConsumer{
		store:                       store,
		runner:                      mockNotifier,
		logger:                      log.NewNopLogger(),
		notificationRetentionPeriod: defaultRetentionPeriod,
	}

	// Send one notification that we haven't seen before
	testId := ulid.New()
	testTitle := fmt.Sprintf("Test title @ %d - %s", time.Now().UnixMicro(), testId)
	testBody := fmt.Sprintf("Test body @ %d - %s", time.Now().UnixMicro(), testId)
	testNotifications := []notify.Notification{
		{
			Title:      testTitle,
			Body:       testBody,
			ID:         testId,
			ValidUntil: getValidUntil(),
		},
	}
	testNotificationsRaw, err := json.Marshal(testNotifications)
	require.NoError(t, err)
	testNotificationsData := bytes.NewReader(testNotificationsRaw)

	// Expect that the notifier is called once to send the one notification successfully
	mockNotifier.On("SendNotification", mock.Anything).Return(nil)

	// Call update and assert our expectations about sent notifications
	err = testNc.Update(testNotificationsData)
	require.NoError(t, err)
	mockNotifier.AssertNumberOfCalls(t, "SendNotification", 1)
}

func TestUpdate_ValidatesNotifications(t *testing.T) {
	t.Parallel()

	store := setupStorage(t)
	mockNotifier := newNotifierMock()
	testNc := &NotificationConsumer{
		store:                       store,
		runner:                      mockNotifier,
		logger:                      log.NewNopLogger(),
		notificationRetentionPeriod: defaultRetentionPeriod,
	}

	tests := []struct {
		testNotification notify.Notification
		name             string
	}{
		{
			name: "Invalid because title and body are empty",
			testNotification: notify.Notification{
				Title:      "",
				Body:       "",
				ID:         ulid.New(),
				ValidUntil: getValidUntil(),
			},
		},
		{
			name: "Invalid because the notification is expired",
			testNotification: notify.Notification{
				Title:      "Expired notification",
				Body:       "Expired notification body",
				ID:         ulid.New(),
				ValidUntil: time.Now().Add(-1 * time.Hour).Unix(),
			},
		},
		{
			name: "Invalid because the action URI is not a real URI",
			testNotification: notify.Notification{
				Title:      "Test notification",
				Body:       "This notification has an action URI that is not valid",
				ID:         ulid.New(),
				ValidUntil: getValidUntil(),
				ActionUri:  "some_thing:foo/bar",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testNotifications := []notify.Notification{tt.testNotification}
			testNotificationsRaw, err := json.Marshal(testNotifications)
			require.NoError(t, err)
			testNotificationsData := bytes.NewReader(testNotificationsRaw)

			// Call update and assert our expectations about sent notifications
			err = testNc.Update(testNotificationsData)
			require.NoError(t, err)
			mockNotifier.AssertNumberOfCalls(t, "SendNotification", 0)
		})
	}
}

func TestUpdate_HandlesDuplicates(t *testing.T) {
	t.Parallel()

	store := setupStorage(t)
	mockNotifier := newNotifierMock()
	testNc := &NotificationConsumer{
		store:                       store,
		runner:                      mockNotifier,
		logger:                      log.NewNopLogger(),
		notificationRetentionPeriod: defaultRetentionPeriod,
	}

	// Queue up two duplicate notifications
	testId := ulid.New()
	testTitle := fmt.Sprintf("Test title @ %d - %s", time.Now().UnixMicro(), testId)
	testBody := fmt.Sprintf("Test body @ %d - %s", time.Now().UnixMicro(), testId)
	testNotifications := []notify.Notification{
		{
			Title:      testTitle,
			Body:       testBody,
			ID:         testId,
			ValidUntil: getValidUntil(),
		},
		{
			Title:      testTitle,
			Body:       testBody,
			ID:         testId,
			ValidUntil: getValidUntil(),
		},
	}
	testNotificationsRaw, err := json.Marshal(testNotifications)
	require.NoError(t, err)
	testNotificationsData := bytes.NewReader(testNotificationsRaw)

	// Expect that the notifier is called only once, to send the first notification
	mockNotifier.On("SendNotification", mock.Anything).Return(nil)

	// Call update and assert our expectations about sent notifications
	err = testNc.Update(testNotificationsData)
	require.NoError(t, err)
	mockNotifier.AssertNumberOfCalls(t, "SendNotification", 1)
}

func TestUpdate_HandlesDuplicatesWhenFirstNotificationCouldNotBeSent(t *testing.T) {
	t.Parallel()

	store := setupStorage(t)
	mockNotifier := newNotifierMock()
	testNc := &NotificationConsumer{
		store:                       store,
		runner:                      mockNotifier,
		logger:                      log.NewNopLogger(),
		notificationRetentionPeriod: defaultRetentionPeriod,
	}

	// Queue up two duplicate notifications
	testId := ulid.New()
	testTitle := fmt.Sprintf("Test title @ %d - %s", time.Now().UnixMicro(), testId)
	testBody := fmt.Sprintf("Test body @ %d - %s", time.Now().UnixMicro(), testId)
	testNotifications := []notify.Notification{
		{
			Title:      testTitle,
			Body:       testBody,
			ID:         testId,
			ValidUntil: getValidUntil(),
		},
		{
			Title:      testTitle,
			Body:       testBody,
			ID:         testId,
			ValidUntil: getValidUntil(),
		},
	}
	testNotificationsRaw, err := json.Marshal(testNotifications)
	require.NoError(t, err)
	testNotificationsData := bytes.NewReader(testNotificationsRaw)

	// Expect that the notifier is called twice: once to unsuccessfully send the first notification, and again to send the duplicate successfully
	errorCall := mockNotifier.On("SendNotification", mock.Anything).Return(errors.New("test error"))
	mockNotifier.On("SendNotification", mock.Anything).Return(nil).NotBefore(errorCall)

	// Call update and assert our expectations about sent notifications
	err = testNc.Update(testNotificationsData)
	require.NoError(t, err)
	mockNotifier.AssertNumberOfCalls(t, "SendNotification", 2)
}

func TestCleanup(t *testing.T) {
	t.Parallel()

	store := setupStorage(t)
	testNc := &NotificationConsumer{
		store:                       store,
		runner:                      newNotifierMock(),
		logger:                      log.NewNopLogger(),
		notificationRetentionPeriod: defaultRetentionPeriod,
	}

	// Save two entries in the db -- one sent a year ago, and one sent now.
	notificationIdToDelete := ulid.New()
	testNc.markNotificationSent(notify.Notification{
		Title:  "Some old test title",
		Body:   "Some old test body",
		ID:     notificationIdToDelete,
		SentAt: time.Now().Add(-365 * 24 * time.Hour),
	})
	notificationIdToRetain := ulid.New()
	testNc.markNotificationSent(notify.Notification{
		Title:  "Some new test title",
		Body:   "Some new test body",
		ID:     notificationIdToRetain,
		SentAt: time.Now(),
	})

	// Confirm we have both entries in the db.
	oldNotificationRecord, err := store.Get([]byte(notificationIdToDelete))
	require.NotNil(t, oldNotificationRecord, "old notification was not seeded in db")
	require.NoError(t, err)

	newNotificationRecord, err := store.Get([]byte(notificationIdToRetain))
	require.NotNil(t, newNotificationRecord, "new notification was not seeded in db")
	require.NoError(t, err)

	// Now, run cleanup.
	testNc.cleanup()

	// Confirm that the old notification record was deleted, and the new one was not.
	oldNotificationRecord, err = store.Get([]byte(notificationIdToDelete))
	require.Nil(t, oldNotificationRecord, "old notification was not cleaned up but should have been")
	require.NoError(t, err)

	newNotificationRecord, err = store.Get([]byte(notificationIdToRetain))
	require.NotNil(t, newNotificationRecord, "new notification was cleaned up but should not have been")
	require.NoError(t, err)
}

func TestUpdate_HandlesMalformedNotifications(t *testing.T) {
	t.Parallel()

	store := setupStorage(t)
	mockNotifier := newNotifierMock()
	testNc := &NotificationConsumer{
		store:                       store,
		runner:                      mockNotifier,
		logger:                      log.NewNopLogger(),
		notificationRetentionPeriod: defaultRetentionPeriod,
	}

	// Queue up two notifications -- one malformed, one correctly formed
	testId := ulid.New()
	goodNotification := notify.Notification{
		Title:      fmt.Sprintf("Test title @ %d - %s", time.Now().UnixMicro(), testId),
		Body:       fmt.Sprintf("Test body @ %d - %s", time.Now().UnixMicro(), testId),
		ID:         testId,
		ValidUntil: getValidUntil(),
	}
	goodNotificationRaw, err := json.Marshal(goodNotification)
	require.NoError(t, err)
	badNotification := struct {
		AnUnknownField      string `json:"an_unknown_field"`
		AnotherUnknownField bool   `json:"another_unknown_field"`
	}{
		AnUnknownField:      testId,
		AnotherUnknownField: true,
	}
	badNotificationRaw, err := json.Marshal(badNotification)
	require.NoError(t, err)
	testNotifications := []json.RawMessage{goodNotificationRaw, badNotificationRaw}
	testNotificationsRaw, err := json.Marshal(testNotifications)
	require.NoError(t, err)
	testNotificationsData := bytes.NewReader(testNotificationsRaw)

	// Expect that the notifier is still called once, to send the good notification
	mockNotifier.On("SendNotification", goodNotification).Return(nil)

	// Call update and assert our expectations about sent notifications
	err = testNc.Update(testNotificationsData)
	require.NoError(t, err)
	mockNotifier.AssertExpectations(t)
	mockNotifier.AssertNumberOfCalls(t, "SendNotification", 1)
}

func setupStorage(t *testing.T) types.KVStore {
	s, err := storageci.NewStore(t, log.NewNopLogger(), storage.SentNotificationsStore.String())
	require.NoError(t, err)
	return s
}

func getValidUntil() int64 {
	return time.Now().Add(1 * time.Hour).Unix()
}
