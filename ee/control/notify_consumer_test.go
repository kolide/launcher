package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
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
	testTitle := fmt.Sprintf("Test title @ %d - 9779e3b9-75e6-4b59-8ed6-5a2d6d2934ea", time.Now().UnixMicro())
	testBody := fmt.Sprintf("Test body @ %d - 9779e3b9-75e6-4b59-8ed6-5a2d6d2934ea", time.Now().UnixMicro())
	testNotifications := []notification{
		{
			Title: testTitle,
			Body:  testBody,
			UUID:  "9779e3b9-75e6-4b59-8ed6-5a2d6d2934ea",
		},
	}
	testNotificationsRaw, err := json.Marshal(testNotifications)
	require.NoError(t, err)
	testNotificationsData := bytes.NewReader(testNotificationsRaw)

	// Expect that the notifier is called once to send the one notification successfully
	mockNotifier.On("SendNotification", testTitle, testBody).Return(nil)

	// Call update and assert our expectations about sent notifications
	testNc.Update(testNotificationsData)
	mockNotifier.AssertCalled(t, "SendNotification", testTitle, testBody)
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
	testTitle := fmt.Sprintf("Test title @ %d - b4ea464c-58c1-4fa3-ada2-57fe9e3a057d", time.Now().UnixMicro())
	testBody := fmt.Sprintf("Test body @ %d - b4ea464c-58c1-4fa3-ada2-57fe9e3a057d", time.Now().UnixMicro())
	testUUID := "b4ea464c-58c1-4fa3-ada2-57fe9e3a057d"
	testNotifications := []notification{
		{
			Title: testTitle,
			Body:  testBody,
			UUID:  testUUID,
		},
		{
			Title: testTitle,
			Body:  testBody,
			UUID:  testUUID,
		},
	}
	testNotificationsRaw, err := json.Marshal(testNotifications)
	require.NoError(t, err)
	testNotificationsData := bytes.NewReader(testNotificationsRaw)

	// Expect that the notifier is called only once, to send the first notification
	mockNotifier.On("SendNotification", testTitle, testBody).Return(nil)

	// Call update and assert our expectations about sent notifications
	testNc.Update(testNotificationsData)
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
	testTitle := fmt.Sprintf("Test title @ %d - 071ef96d-c66a-4d3d-8143-74fe11e04bae", time.Now().UnixMicro())
	testBody := fmt.Sprintf("Test body @ %d - 071ef96d-c66a-4d3d-8143-74fe11e04bae", time.Now().UnixMicro())
	testUUID := "071ef96d-c66a-4d3d-8143-74fe11e04bae"
	testNotifications := []notification{
		{
			Title: testTitle,
			Body:  testBody,
			UUID:  testUUID,
		},
		{
			Title: testTitle,
			Body:  testBody,
			UUID:  testUUID,
		},
	}
	testNotificationsRaw, err := json.Marshal(testNotifications)
	require.NoError(t, err)
	testNotificationsData := bytes.NewReader(testNotificationsRaw)

	// Expect that the notifier is called twice: once to unsuccessfully send the first notification, and again to send the duplicate successfully
	errorCall := mockNotifier.On("SendNotification", testTitle, testBody).Return(errors.New("test error"))
	mockNotifier.On("SendNotification", testTitle, testBody).Return(nil).NotBefore(errorCall)

	// Call update and assert our expectations about sent notifications
	testNc.Update(testNotificationsData)
	mockNotifier.AssertNumberOfCalls(t, "SendNotification", 2)
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
