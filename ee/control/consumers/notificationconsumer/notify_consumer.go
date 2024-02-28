package notificationconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/url"

	"github.com/kolide/launcher/ee/agent/types"
	desktopRunner "github.com/kolide/launcher/ee/desktop/runner"
	"github.com/kolide/launcher/ee/desktop/user/notify"
)

// Consumes notifications from control server
type NotificationConsumer struct {
	runner  userProcessesRunner
	slogger *slog.Logger
}

// The desktop runner fullfils this interface -- it exists for testing purposes.
type userProcessesRunner interface {
	SendNotification(n notify.Notification) error
}

const (
	// Identifier for this consumer.
	NotificationSubsystem = "desktop_notifier"
)

type notificationConsumerOption func(*NotificationConsumer)

func NewNotifyConsumer(ctx context.Context, k types.Knapsack, runner *desktopRunner.DesktopUsersProcessesRunner, opts ...notificationConsumerOption) (*NotificationConsumer, error) {
	nc := &NotificationConsumer{
		runner:  runner,
		slogger: k.Slogger().With("component", NotificationSubsystem),
	}

	for _, opt := range opts {
		opt(nc)
	}

	return nc, nil
}

func (nc *NotificationConsumer) Do(data io.Reader) error {
	if nc == nil {
		return errors.New("NotificationConsumer is nil")
	}

	var notification notify.Notification
	if err := json.NewDecoder(data).Decode(&notification); err != nil {
		nc.slogger.Log(context.TODO(), slog.LevelWarn,
			"received notification in unexpected format from K2, discarding",
			"err", err,
		)
		return nil
	}

	if !nc.notificationIsValid(notification) {
		return nil
	}

	return nc.runner.SendNotification(notification)
}

func (nc *NotificationConsumer) notificationIsValid(notificationToCheck notify.Notification) bool {
	// If action URI is set, it must be a valid URI
	if notificationToCheck.ActionUri != "" {
		_, err := url.Parse(notificationToCheck.ActionUri)
		if err != nil {
			nc.slogger.Log(context.TODO(), slog.LevelWarn,
				"received invalid action_uri from K2",
				"notification_id", notificationToCheck.ID,
				"action_uri", notificationToCheck.ActionUri,
				"err", err,
			)
			return false
		}
	}

	// Notification must not be blank
	return notificationToCheck.Title != "" && notificationToCheck.Body != ""
}
