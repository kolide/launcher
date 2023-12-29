package notificationconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	desktopRunner "github.com/kolide/launcher/ee/desktop/runner"
	"github.com/kolide/launcher/ee/desktop/user/notify"
)

// Consumes notifications from control server
type NotificationConsumer struct {
	runner userProcessesRunner
	logger log.Logger
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

func WithLogger(logger log.Logger) notificationConsumerOption {
	return func(nc *NotificationConsumer) {
		nc.logger = log.With(logger,
			"component", NotificationSubsystem,
		)
	}
}

func NewNotifyConsumer(ctx context.Context, runner *desktopRunner.DesktopUsersProcessesRunner, opts ...notificationConsumerOption) (*NotificationConsumer, error) {
	nc := &NotificationConsumer{
		runner: runner,
		logger: log.NewNopLogger(),
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
		level.Debug(nc.logger).Log("msg", "received notification in unexpected format from K2, discarding", "err", err)
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
			level.Debug(nc.logger).Log(
				"msg", "received invalid action_uri from K2",
				"notification_id", notificationToCheck.ID,
				"action_uri", notificationToCheck.ActionUri,
				"err", err)
			return false
		}
	}

	// Notification must not be blank
	return notificationToCheck.Title != "" && notificationToCheck.Body != ""
}
