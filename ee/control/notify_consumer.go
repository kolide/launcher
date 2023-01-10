package control

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	desktopRunner "github.com/kolide/launcher/ee/desktop/runner"
	"github.com/kolide/launcher/pkg/osquery"
	"go.etcd.io/bbolt"
)

// Consumes notifications from control server, tracks when notifications are sent to end user
type NotificationConsumer struct {
	db              *bbolt.DB
	runner          notifier
	logger          log.Logger
	notificationTtl time.Duration // How long until a notification expires -- i.e. how long until we can re-send that identical notification
}

// The desktop runner fullfils this interface -- it exists primarily for testing purposes.
type notifier interface {
	SendNotification(title, body string) error
}

// Represents notification received from control server; SentAt is set by this consumer after sending.
type notification struct {
	Title  string    `json:"title"`
	Body   string    `json:"body"`
	UUID   string    `json:"uuid"`
	SentAt time.Time `json:"sent_at,omitempty"`
}

// Identifier for this consumer.
const NotificationSubsystem = "desktop_notifier"

type notificationConsumerOption func(*NotificationConsumer)

func WithLogger(logger log.Logger) notificationConsumerOption {
	return func(nc *NotificationConsumer) {
		nc.logger = log.With(logger,
			"component", NotificationSubsystem,
		)
	}
}

func WithNotificationTtl(ttl time.Duration) notificationConsumerOption {
	return func(nc *NotificationConsumer) {
		nc.notificationTtl = ttl
	}
}

func NewNotifyConsumer(db *bbolt.DB, runner *desktopRunner.DesktopUsersProcessesRunner, opts ...notificationConsumerOption) (*NotificationConsumer, error) {
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
		db:              db,
		runner:          runner,
		logger:          log.NewNopLogger(),
		notificationTtl: time.Hour * 1,
	}

	for _, opt := range opts {
		opt(nc)
	}

	return nc, nil
}

func (nc *NotificationConsumer) Update(data io.Reader) error {
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
	if nc.notificationAlreadySent(notificationToSend) {
		level.Debug(nc.logger).Log("msg", "received duplicate notification", "title", notificationToSend.Title)
		return
	}

	if err := nc.runner.SendNotification(notificationToSend.Title, notificationToSend.Body); err != nil {
		level.Error(nc.logger).Log("msg", "could not send notification", "title", notificationToSend.Title, "err", err)
		return
	}

	notificationToSend.SentAt = time.Now()
	nc.markNotificationSent(notificationToSend)
}

func (nc *NotificationConsumer) notificationAlreadySent(notificationToCheck notification) bool {
	alreadySent := false

	if err := nc.db.View(func(tx *bbolt.Tx) error {
		sentNotificationRaw := tx.Bucket([]byte(osquery.SentNotificationsBucket)).Get(notificationKey(notificationToCheck))
		if sentNotificationRaw == nil {
			// No previous record -- notification has not been sent before
			return nil
		}

		var previouslySentNotification notification
		if err := json.Unmarshal(sentNotificationRaw, &previouslySentNotification); err != nil {
			return fmt.Errorf("could not unmarshal previously sent notification: %w", err)
		}

		// Check to see how long ago the previously-sent notification was sent -- if it's within the
		// configured TTL, then we consider it a duplicate and will not re-send.
		sentExpiredAt := previouslySentNotification.SentAt.Add(nc.notificationTtl)
		alreadySent = sentExpiredAt.After(time.Now())

		return nil
	}); err != nil {
		level.Error(nc.logger).Log("msg", "could not read sent notifications from bucket", "err", err)
	}

	return alreadySent
}

func (nc *NotificationConsumer) markNotificationSent(sentNotification notification) {
	if err := nc.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(osquery.SentNotificationsBucket))
		if err != nil {
			return fmt.Errorf("could not create bucket: %w", err)
		}

		rawNotification, err := json.Marshal(sentNotification)
		if err != nil {
			return fmt.Errorf("could not marshal sent notification: %w", err)
		}

		if err := b.Put(notificationKey(sentNotification), rawNotification); err != nil {
			return fmt.Errorf("could not write to key: %w", err)
		}

		return nil
	}); err != nil {
		level.Error(nc.logger).Log("msg", "could not mark notification sent", "title", sentNotification.Title, "err", err)
	}
}

func notificationKey(n notification) []byte {
	combined := fmt.Sprintf("%d:%s #### %d:%s", len(n.Title), n.Title, len(n.Body), n.Body)
	h := sha256.New()
	h.Write([]byte(combined))
	key := fmt.Sprintf("%x", h.Sum(nil))
	return []byte(key)
}
