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
		db:              db,
		runner:          mockNotifier,
		logger:          log.NewNopLogger(),
		notificationTtl: time.Hour * 1,
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
		db:              db,
		runner:          mockNotifier,
		logger:          log.NewNopLogger(),
		notificationTtl: time.Hour * 1,
	}

	// Queue up a bunch of invalid notifications
	testNotifications := []notification{
		// Invalid because the title and body are empty
		{
			Title:      "",
			Body:       "",
			ID:         ulid.New(),
			ValidUntil: getValidUntil(),
		},
		// Invalid because `ValidUntil` isn't a timestamp
		{
			Title:      "Test title 1",
			Body:       "Test body 1",
			ID:         ulid.New(),
			ValidUntil: "some time in the future",
		},
		// Invalid because `ValidUntil` is an unexpected format
		{
			Title:      "Test title 1",
			Body:       "Test body 1",
			ID:         ulid.New(),
			ValidUntil: time.Now().Add(1 * time.Hour).Format(time.RFC1123),
		},
		// Invalid because it's expired
		{
			Title:      "Test title 1",
			Body:       "Test body 1",
			ID:         ulid.New(),
			ValidUntil: time.Now().Add(-1 * time.Hour).Format(iso8601Format),
		},
	}
	testNotificationsRaw, err := json.Marshal(testNotifications)
	require.NoError(t, err)
	testNotificationsData := bytes.NewReader(testNotificationsRaw)

	// Call update and assert our expectations about sent notifications
	err = testNc.Update(testNotificationsData)
	require.NoError(t, err)
	mockNotifier.AssertNumberOfCalls(t, "SendNotification", 0)
}

func TestUpdate_HandlesDuplicates(t *testing.T) {
	t.Parallel()

	db := setUpDb(t)
	mockNotifier := newNotifierMock()
	testNc := &NotificationConsumer{
		db:              db,
		runner:          mockNotifier,
		logger:          log.NewNopLogger(),
		notificationTtl: time.Hour * 1,
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
		db:              db,
		runner:          mockNotifier,
		logger:          log.NewNopLogger(),
		notificationTtl: time.Hour * 1,
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

func TestUpdate_ResendsOnceTTLExpires(t *testing.T) {
	t.Parallel()

	db := setUpDb(t)
	mockNotifier := newNotifierMock()
	testNc := &NotificationConsumer{
		db:              db,
		runner:          mockNotifier,
		logger:          log.NewNopLogger(),
		notificationTtl: time.Hour * 1,
	}

	// Queue up one notification
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

	// Expect that the notifier is called once to send the one notification
	mockNotifier.On("SendNotification", testTitle, testBody).Return(nil)

	// Call update and assert our expectations about sent notifications
	expectedCalls := 1
	testNotificationsRaw1, err := json.Marshal(testNotifications)
	require.NoError(t, err)
	testNotificationsData1 := bytes.NewReader(testNotificationsRaw1)
	err = testNc.Update(testNotificationsData1)
	require.NoError(t, err)
	mockNotifier.AssertNumberOfCalls(t, "SendNotification", expectedCalls)

	// Try again and confirm that it isn't re-sent
	testNotificationsRaw2, err := json.Marshal(testNotifications)
	require.NoError(t, err)
	testNotificationsData2 := bytes.NewReader(testNotificationsRaw2)
	err = testNc.Update(testNotificationsData2)
	require.NoError(t, err)
	mockNotifier.AssertNumberOfCalls(t, "SendNotification", expectedCalls)

	// Set the TTL to 0 so that the notification counts as expired
	testNc.notificationTtl = time.Microsecond * 1
	time.Sleep(5 * time.Microsecond) // wait, just to be sure

	// Try to send the notificatio again and confirm it's re-sent
	expectedCalls += 1
	testNotificationsRaw3, err := json.Marshal(testNotifications)
	require.NoError(t, err)
	testNotificationsData3 := bytes.NewReader(testNotificationsRaw3)
	err = testNc.Update(testNotificationsData3)
	require.NoError(t, err)
	mockNotifier.AssertNumberOfCalls(t, "SendNotification", expectedCalls)
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

func getValidUntil() string {
	return time.Now().Add(1 * time.Hour).Format(iso8601Format)
}
