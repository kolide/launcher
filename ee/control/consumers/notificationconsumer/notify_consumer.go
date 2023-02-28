package notificationconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
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
	SendNotification(title, body string) error
}

// Represents notification received from control server; SentAt is set by this consumer after sending.
// For the time being, notifications are per-end user device and not per-user.
type notification struct {
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	ID         string    `json:"id"`
	ValidUntil int64     `json:"valid_until"` // timestamp
	SentAt     time.Time `json:"sent_at,omitempty"`
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

	var notificationsToProcess []notification
	if err := json.NewDecoder(data).Decode(&notificationsToProcess); err != nil {
		return fmt.Errorf("failed to decode notification data: %w", err)
	}

	for _, notificationToProcess := range notificationsToProcess {
		nc.notify(notificationToProcess)
	}

	return nil
}

func (nc *NotificationConsumer) notify(notificationToSend notification) {
	if !nc.notificationIsValid(notificationToSend) {
		return
	}

	if nc.notificationAlreadySent(notificationToSend) {
		return
	}

	if err := nc.runner.SendNotification(notificationToSend.Title, notificationToSend.Body); err != nil {
		level.Error(nc.logger).Log("msg", "could not send notification", "title", notificationToSend.Title, "err", err)
		return
	}

	notificationToSend.SentAt = time.Now()
	nc.markNotificationSent(notificationToSend)
}

func (nc *NotificationConsumer) notificationIsValid(notificationToCheck notification) bool {
	// Check that the notification is not expired --
	validUntil := time.Unix(notificationToCheck.ValidUntil, 0)

	// Notification has expired
	if validUntil.Before(time.Now()) {
		return false
	}

	// Notification must not be blank
	return notificationToCheck.Title != "" && notificationToCheck.Body != ""
}

func (nc *NotificationConsumer) notificationAlreadySent(notificationToCheck notification) bool {
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

func (nc *NotificationConsumer) markNotificationSent(sentNotification notification) {
	rawNotification, err := json.Marshal(sentNotification)
	if err != nil {
		level.Error(nc.logger).Log("msg", "could not marshal sent notification", "title", sentNotification.Title, "err", err)
		return
	}

	if err := nc.store.Set([]byte(sentNotification.ID), rawNotification); err != nil {
		level.Error(nc.logger).Log("msg", "could not mark notification sent", "title", sentNotification.Title, "err", err)
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
	if err := nc.store.ForEach(func(k, v []byte) error {
		var sentNotification notification
		if err := json.Unmarshal(v, &sentNotification); err != nil {
			return fmt.Errorf("error processing %s: %w", string(k), err)
		}

		if sentNotification.SentAt.Add(nc.notificationRetentionPeriod).Before(time.Now()) {
			keysToDelete = append(keysToDelete, k)
		}

		return nil
	}); err != nil {
		level.Error(nc.logger).Log("msg", "could not iterate over bucket items to determine which are expired", "err", err)
	}

	// Delete all old keys
	for _, k := range keysToDelete {
		if err := nc.store.Delete(k); err != nil {
			// Log, but don't error, since we might be able to delete some others
			level.Error(nc.logger).Log("msg", "could not delete old notification from bucket", "key", string(k), "err", err)
		}
	}
}
