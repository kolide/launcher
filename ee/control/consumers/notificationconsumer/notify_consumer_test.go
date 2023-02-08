package notificationconsumer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

type notifierMock struct{ mock.Mock }

func newNotifierMock() *notifierMock { return &notifierMock{} }

func (nm *notifierMock) SendNotification(title, body string) error {
	args := nm.Called(title, body)
	return args.Error(0)
}

func TestUpdate_HappyPath(t *testing.T) {
	t.Parallel()

	db := setUpDb(t)
	mockNotifier := newNotifierMock()
	testNc := &NotificationConsumer{
		db:                          db,
		runner:                      mockNotifier,
		logger:                      log.NewNopLogger(),
		notificationRetentionPeriod: defaultRetentionPeriod,
	}

	// Send one notification that we haven't seen before
	testId := ulid.New()
	testTitle := fmt.Sprintf("Test title @ %d - %s", time.Now().UnixMicro(), testId)
	testBody := fmt.Sprintf("Test body @ %d - %s", time.Now().UnixMicro(), testId)
	testNotifications := []notification{
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
	mockNotifier.On("SendNotification", testTitle, testBody).Return(nil)

	// Call update and assert our expectations about sent notifications
	err = testNc.Update(testNotificationsData)
	require.NoError(t, err)
	mockNotifier.AssertNumberOfCalls(t, "SendNotification", 1)
}

func TestUpdate_ValidatesNotifications(t *testing.T) {
	t.Parallel()

	db := setUpDb(t)
	mockNotifier := newNotifierMock()
	testNc := &NotificationConsumer{
		db:                          db,
		runner:                      mockNotifier,
		logger:                      log.NewNopLogger(),
		notificationRetentionPeriod: defaultRetentionPeriod,
	}

	tests := []struct {
		testNotification notification
		name             string
	}{
		{
			name: "Invalid because title and body are empty",
			testNotification: notification{
				Title:      "",
				Body:       "",
				ID:         ulid.New(),
				ValidUntil: getValidUntil(),
			},
		},
		{
			name: "Invalid because the notification is expired",
			testNotification: notification{
				Title:      "Expired notification",
				Body:       "Expired notification body",
				ID:         ulid.New(),
				ValidUntil: time.Now().Add(-1 * time.Hour).Unix(),
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testNotifications := []notification{tt.testNotification}
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

	db := setUpDb(t)
	mockNotifier := newNotifierMock()
	testNc := &NotificationConsumer{
		db:                          db,
		runner:                      mockNotifier,
		logger:                      log.NewNopLogger(),
		notificationRetentionPeriod: defaultRetentionPeriod,
	}

	// Queue up two duplicate notifications
	testId := ulid.New()
	testTitle := fmt.Sprintf("Test title @ %d - %s", time.Now().UnixMicro(), testId)
	testBody := fmt.Sprintf("Test body @ %d - %s", time.Now().UnixMicro(), testId)
	testNotifications := []notification{
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
	mockNotifier.On("SendNotification", testTitle, testBody).Return(nil)

	// Call update and assert our expectations about sent notifications
	err = testNc.Update(testNotificationsData)
	require.NoError(t, err)
	mockNotifier.AssertNumberOfCalls(t, "SendNotification", 1)
}

func TestUpdate_HandlesDuplicatesWhenFirstNotificationCouldNotBeSent(t *testing.T) {
	t.Parallel()

	db := setUpDb(t)
	mockNotifier := newNotifierMock()
	testNc := &NotificationConsumer{
		db:                          db,
		runner:                      mockNotifier,
		logger:                      log.NewNopLogger(),
		notificationRetentionPeriod: defaultRetentionPeriod,
	}

	// Queue up two duplicate notifications
	testId := ulid.New()
	testTitle := fmt.Sprintf("Test title @ %d - %s", time.Now().UnixMicro(), testId)
	testBody := fmt.Sprintf("Test body @ %d - %s", time.Now().UnixMicro(), testId)
	testNotifications := []notification{
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
	errorCall := mockNotifier.On("SendNotification", testTitle, testBody).Return(errors.New("test error"))
	mockNotifier.On("SendNotification", testTitle, testBody).Return(nil).NotBefore(errorCall)

	// Call update and assert our expectations about sent notifications
	err = testNc.Update(testNotificationsData)
	require.NoError(t, err)
	mockNotifier.AssertNumberOfCalls(t, "SendNotification", 2)
}

func TestCleanup(t *testing.T) {
	t.Parallel()

	db := setUpDb(t)
	testNc := &NotificationConsumer{
		db:                          db,
		runner:                      newNotifierMock(),
		logger:                      log.NewNopLogger(),
		notificationRetentionPeriod: defaultRetentionPeriod,
	}

	// Save two entries in the db -- one sent a year ago, and one sent now.
	notificationIdToDelete := ulid.New()
	testNc.markNotificationSent(notification{
		Title:  "Some old test title",
		Body:   "Some old test body",
		ID:     notificationIdToDelete,
		SentAt: time.Now().Add(-365 * 24 * time.Hour),
	})
	notificationIdToRetain := ulid.New()
	testNc.markNotificationSent(notification{
		Title:  "Some new test title",
		Body:   "Some new test body",
		ID:     notificationIdToRetain,
		SentAt: time.Now(),
	})

	// Confirm we have both entries in the db.
	if err := db.View(func(tx *bbolt.Tx) error {
		oldNotificationRecord := tx.Bucket([]byte(osquery.SentNotificationsBucket)).Get([]byte(notificationIdToDelete))
		require.NotNil(t, oldNotificationRecord, "old notification was not seeded in db")

		newNotificationRecord := tx.Bucket([]byte(osquery.SentNotificationsBucket)).Get([]byte(notificationIdToRetain))
		require.NotNil(t, newNotificationRecord, "new notification was not seeded in db")
		return nil
	}); err != nil {
		require.NoError(t, err)
	}

	// Now, run cleanup.
	testNc.cleanup()

	// Confirm that the old notification record was deleted, and the new one was not.
	if err := db.View(func(tx *bbolt.Tx) error {
		oldNotificationRecord := tx.Bucket([]byte(osquery.SentNotificationsBucket)).Get([]byte(notificationIdToDelete))
		require.Nil(t, oldNotificationRecord, "old notification was not cleaned up but should have been")

		newNotificationRecord := tx.Bucket([]byte(osquery.SentNotificationsBucket)).Get([]byte(notificationIdToRetain))
		require.NotNil(t, newNotificationRecord, "new notification was cleaned up but should not have been")
		return nil
	}); err != nil {
		require.NoError(t, err)
	}
}

func setUpDb(t *testing.T) *bbolt.DB {
	// Create a temp directory to hold our bbolt db
	dbDir := t.TempDir()

	// Create database; ensure we clean it up after the test
	db, err := bbolt.Open(filepath.Join(dbDir, "notifier_test.db"), 0600, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	// Create the bucket
	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(osquery.SentNotificationsBucket))
		require.NoError(t, err)
		return nil
	})
	require.NoError(t, err)

	return db
}

func getValidUntil() int64 {
	return time.Now().Add(1 * time.Hour).Unix()
}
