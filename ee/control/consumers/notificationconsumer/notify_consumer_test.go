package notificationconsumer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/desktop/user/notify"
	"github.com/kolide/launcher/pkg/log/multislogger"
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

	mockNotifier := newNotifierMock()

	testNc := &NotificationConsumer{
		runner:  mockNotifier,
		slogger: multislogger.NewNopLogger(),
	}

	// Send one notification
	testId := ulid.New()
	testTitle := fmt.Sprintf("Test title @ %d - %s", time.Now().UnixMicro(), testId)
	testBody := fmt.Sprintf("Test body @ %d - %s", time.Now().UnixMicro(), testId)
	testActionUri := "https://www.kolide.com"
	testNotification := notify.Notification{
		Title:      testTitle,
		Body:       testBody,
		ID:         testId,
		ValidUntil: getValidUntil(),
		ActionUri:  testActionUri,
	}

	testNotificationsRaw, err := json.Marshal(testNotification)
	require.NoError(t, err)
	testNotificationsData := bytes.NewReader(testNotificationsRaw)

	// Expect that the notifier is called once to send the one notification successfully
	mockNotifier.On("SendNotification", mock.Anything).Return(nil)

	// Call update and assert our expectations about sent notifications
	err = testNc.Do(testNotificationsData)
	require.NoError(t, err)
	mockNotifier.AssertNumberOfCalls(t, "SendNotification", 1)
}

func TestUpdate_HappyPath_NoAction(t *testing.T) {
	t.Parallel()

	mockNotifier := newNotifierMock()
	testNc := &NotificationConsumer{
		runner:  mockNotifier,
		slogger: multislogger.NewNopLogger(),
	}

	// Send one notification that we haven't seen before
	testId := ulid.New()
	testTitle := fmt.Sprintf("Test title @ %d - %s", time.Now().UnixMicro(), testId)
	testBody := fmt.Sprintf("Test body @ %d - %s", time.Now().UnixMicro(), testId)
	testNotification := notify.Notification{
		Title:      testTitle,
		Body:       testBody,
		ID:         testId,
		ValidUntil: getValidUntil(),
	}
	testNotificationsRaw, err := json.Marshal(testNotification)
	require.NoError(t, err)
	testNotificationsData := bytes.NewReader(testNotificationsRaw)

	// Expect that the notifier is called once to send the one notification successfully
	mockNotifier.On("SendNotification", mock.Anything).Return(nil)

	// Call update and assert our expectations about sent notifications
	err = testNc.Do(testNotificationsData)
	require.NoError(t, err)
	mockNotifier.AssertNumberOfCalls(t, "SendNotification", 1)
}

func TestUpdate_ValidatesNotifications(t *testing.T) {
	t.Parallel()

	mockNotifier := newNotifierMock()
	testNc := &NotificationConsumer{
		runner:  mockNotifier,
		slogger: multislogger.NewNopLogger(),
	}

	tests := []struct {
		testNotification notify.Notification
		name             string
	}{
		{
			name: "Invalid because title and body are empty",
			testNotification: notify.Notification{
				Title: "",
				Body:  "",
			},
		},
		{
			name: "Invalid because the action URI is not a real URI",
			testNotification: notify.Notification{
				Title:     "Test notification",
				Body:      "This notification has an action URI that is not valid",
				ActionUri: "some_thing:foo/bar",
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
			err = testNc.Do(testNotificationsData)
			require.NoError(t, err)
			mockNotifier.AssertNumberOfCalls(t, "SendNotification", 0)
		})
	}
}

func getValidUntil() int64 {
	return time.Now().Add(1 * time.Hour).Unix()
}
