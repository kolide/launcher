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
	"github.com/kolide/launcher/pkg/osquery"
	"go.etcd.io/bbolt"
)

// Consumes notifications from control server, tracks when notifications are sent to end user
type NotificationConsumer struct {
	db                          *bbolt.DB
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

func NewNotifyConsumer(db *bbolt.DB, runner *desktopRunner.DesktopUsersProcessesRunner, ctx context.Context, opts ...notificationConsumerOption) (*NotificationConsumer, error) {
	// Create the bucket to track sent notifications if it doesn't exist
	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(osquery.SentNotificationsBucket))
		if err != nil {
			return fmt.Errorf("could not create bucket: %w", err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("cannot create notifier without db bucket: %w", err)
	}

	nc := &NotificationConsumer{
		db:                          db,
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
		level.Debug(nc.logger).Log("msg", "could not send notification", "title", notificationToSend.Title, "err", err)
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
	alreadySent := false

	if err := nc.db.View(func(tx *bbolt.Tx) error {
		sentNotificationRaw := tx.Bucket([]byte(osquery.SentNotificationsBucket)).Get([]byte(notificationToCheck.ID))
		if sentNotificationRaw == nil {
			// No previous record -- notification has not been sent before
			return nil
		}

		alreadySent = true

		return nil
	}); err != nil {
		level.Debug(nc.logger).Log("msg", "could not read sent notifications from bucket", "err", err)
	}

	return alreadySent
}

func (nc *NotificationConsumer) markNotificationSent(sentNotification notify.Notification) {
	if err := nc.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(osquery.SentNotificationsBucket))
		if err != nil {
			return fmt.Errorf("could not create bucket: %w", err)
		}

		rawNotification, err := json.Marshal(sentNotification)
		if err != nil {
			return fmt.Errorf("could not marshal sent notification: %w", err)
		}

		if err := b.Put([]byte(sentNotification.ID), rawNotification); err != nil {
			return fmt.Errorf("could not write to key: %w", err)
		}

		return nil
	}); err != nil {
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
	if err := nc.db.View(func(tx *bbolt.Tx) error {
		if err := tx.Bucket([]byte(osquery.SentNotificationsBucket)).ForEach(func(k, v []byte) error {
			var sentNotification notify.Notification
			if err := json.Unmarshal(v, &sentNotification); err != nil {
				return fmt.Errorf("error processing %s: %w", string(k), err)
			}

			if sentNotification.SentAt.Add(nc.notificationRetentionPeriod).Before(time.Now()) {
				keysToDelete = append(keysToDelete, k)
			}

			return nil
		}); err != nil {
			return fmt.Errorf("error iterating over keys in bucket: %w", err)
		}

		return nil
	}); err != nil {
		level.Debug(nc.logger).Log("msg", "could not iterate over bucket items to determine which are expired", "err", err)
	}

	// Delete all old keys
	if err := nc.db.Update(func(tx *bbolt.Tx) error {
		for _, k := range keysToDelete {
			if err := tx.Bucket([]byte(osquery.SentNotificationsBucket)).Delete(k); err != nil {
				// Log, but don't error, since we might be able to delete some others
				level.Debug(nc.logger).Log("msg", "could not delete old notification from bucket", "key", string(k), "err", err)
			}
		}

		return nil
	}); err != nil {
		level.Debug(nc.logger).Log("msg", "could not delete old notifications from bucket", "err", err)
	}
}
