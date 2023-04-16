package notificationconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/ee/desktop/notify"
	desktopRunner "github.com/kolide/launcher/ee/desktop/runner"
	"github.com/kolide/launcher/pkg/agent/types"
)

// Consumes notifications from control server, tracks when notifications are sent to end user
type NotificationConsumer struct {
	store                       types.KVStore
	runner                      userProcessesRunner
	logger                      log.Logger
	notificationRetentionPeriod time.Duration
	cleanupInterval             time.Duration
	ctx                         context.Context
	cancel                      context.CancelFunc
}

// The desktop runner fullfils this interface -- it exists for testing purposes.
type userProcessesRunner interface {
	SendNotification(n notify.Notification) error
}

const (
	// Identifier for this consumer.
	NotificationSubsystem = "desktop_notifier"

	// Approximately 6 months
	defaultRetentionPeriod = time.Hour * 24 * 30 * 6

	// How frequently to check for old notifications
	defaultCleanupInterval = time.Hour * 12
)

type notificationConsumerOption func(*NotificationConsumer)

func WithLogger(logger log.Logger) notificationConsumerOption {
	return func(nc *NotificationConsumer) {
		nc.logger = log.With(logger,
			"component", NotificationSubsystem,
		)
	}
}

func WithNotificationRetentionPeriod(ttl time.Duration) notificationConsumerOption {
	return func(nc *NotificationConsumer) {
		nc.notificationRetentionPeriod = ttl
	}
}

func WithCleanupInterval(cleanupInterval time.Duration) notificationConsumerOption {
	return func(nc *NotificationConsumer) {
		nc.cleanupInterval = cleanupInterval
	}
}

func NewNotifyConsumer(store types.KVStore, runner *desktopRunner.DesktopUsersProcessesRunner, ctx context.Context, opts ...notificationConsumerOption) (*NotificationConsumer, error) {
	nc := &NotificationConsumer{
		store:                       store,
		runner:                      runner,
		logger:                      log.NewNopLogger(),
		notificationRetentionPeriod: defaultRetentionPeriod,
		cleanupInterval:             defaultCleanupInterval,
		ctx:                         ctx,
	}

	for _, opt := range opts {
		opt(nc)
	}

	return nc, nil
}

func (nc *NotificationConsumer) Update(data io.Reader) error {
	if nc == nil {
		return errors.New("NotificationConsumer is nil")
	}

	// We want to unmarshal each notification separately, so that we don't fail to send all notifications
	// if only some are malformed.
	var rawNotificationsToProcess []json.RawMessage
	if err := json.NewDecoder(data).Decode(&rawNotificationsToProcess); err != nil {
		return fmt.Errorf("failed to decode notification data: %w", err)
	}

	for _, rawNotification := range rawNotificationsToProcess {
		var notificationToProcess notify.Notification
		if err := json.Unmarshal(rawNotification, &notificationToProcess); err != nil {
			level.Debug(nc.logger).Log("msg", "received notification in unexpected format from K2, discarding", "err", err)
			continue
		}
		nc.notify(notificationToProcess)
	}

	return nil
}

func (nc *NotificationConsumer) notify(notificationToSend notify.Notification) {
	if !nc.notificationIsValid(notificationToSend) {
		return
	}

	if nc.notificationAlreadySent(notificationToSend) {
		return
	}

	if err := nc.runner.SendNotification(notificationToSend); err != nil {
		// Already logged on desktop side, no need to log again
		return
	}

	notificationToSend.SentAt = time.Now()
	nc.markNotificationSent(notificationToSend)
}

func (nc *NotificationConsumer) notificationIsValid(notificationToCheck notify.Notification) bool {
	// Check that the notification is not expired --
	validUntil := time.Unix(notificationToCheck.ValidUntil, 0)

	// Notification has expired
	if validUntil.Before(time.Now()) {
		return false
	}

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

func (nc *NotificationConsumer) notificationAlreadySent(notificationToCheck notify.Notification) bool {
	sentNotificationRaw, err := nc.store.Get([]byte(notificationToCheck.ID))
	if err != nil {
		level.Error(nc.logger).Log("msg", "could not read sent notifications from bucket", "err", err)
	}

	if sentNotificationRaw == nil {
		// No previous record -- notification has not been sent before
		return false
	}

	return true
}

func (nc *NotificationConsumer) markNotificationSent(sentNotification notify.Notification) {
	rawNotification, err := json.Marshal(sentNotification)
	if err != nil {
		level.Error(nc.logger).Log("msg", "could not marshal sent notification", "title", sentNotification.Title, "err", err)
		return
	}

	if err := nc.store.Set([]byte(sentNotification.ID), rawNotification); err != nil {
		level.Debug(nc.logger).Log("msg", "could not mark notification sent", "title", sentNotification.Title, "err", err)
	}
}

// Runs cleanup job to periodically check for notifications we no longer need to retain and delete them
func (nc *NotificationConsumer) Execute() error {
	nc.runCleanup(nc.ctx)
	return nil
}

// Stops cleanup job
func (nc *NotificationConsumer) Interrupt(err error) {
	nc.cancel()
}

func (nc *NotificationConsumer) runCleanup(ctx context.Context) {
	ctx, nc.cancel = context.WithCancel(ctx)
	t := time.NewTicker(nc.cleanupInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			nc.cleanup()
		}
	}
}

func (nc *NotificationConsumer) cleanup() {
	// Read through all keys in bucket to determine which ones are old enough to be deleted
	keysToDelete := make([][]byte, 0)
	if err := nc.store.ForEach(func(k, v []byte) error {
		var sentNotification notify.Notification
		if err := json.Unmarshal(v, &sentNotification); err != nil {
			return fmt.Errorf("error processing %s: %w", string(k), err)
		}

		if sentNotification.SentAt.Add(nc.notificationRetentionPeriod).Before(time.Now()) {
			keysToDelete = append(keysToDelete, k)
		}

		return nil
	}); err != nil {
		level.Debug(nc.logger).Log("msg", "could not iterate over bucket items to determine which are expired", "err", err)
	}

	// Delete all old keys
	if err := nc.store.Delete(keysToDelete...); err != nil {
		level.Debug(nc.logger).Log("msg", "could not delete old notifications from bucket", "err", err)
	}
}
