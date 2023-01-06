package notify

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/ee/desktop/assets"
	"github.com/kolide/launcher/pkg/osquery"
	"go.etcd.io/bbolt"
)

type Notifier struct {
	db              *bbolt.DB
	logger          log.Logger
	notificationTtl time.Duration
	iconFilepath    string
}

type sentNotification struct {
	Title  string    `json:"title"`
	Body   string    `json:"body"`
	UUID   string    `json:"uuid"`
	SentAt time.Time `json:"sent_at"`
}

type notifierOption func(*Notifier)

func WithLogger(logger log.Logger) notifierOption {
	return func(n *Notifier) {
		n.logger = log.With(logger,
			"component", "desktop_notifier",
		)
	}
}

func WithNotificationTtl(ttl time.Duration) notifierOption {
	return func(n *Notifier) {
		n.notificationTtl = ttl
	}
}

func WithRootDirectory(rootDir string) notifierOption {
	return func(n *Notifier) {
		iconPath, err := setIconPath(rootDir)
		if err != nil {
			level.Error(n.logger).Log("msg", "could not set icon path for notifications", "err", err)
		} else {
			n.iconFilepath = iconPath
		}
	}
}

func setIconPath(rootDir string) (string, error) {
	expectedLocation := filepath.Join(rootDir, assets.KolideIconFilename)

	_, err := os.Stat(expectedLocation)

	if os.IsNotExist(err) {
		if err := os.WriteFile(expectedLocation, assets.KolideDesktopIcon, 0644); err != nil {
			return "", fmt.Errorf("notification icon did not exist; could not create it: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("could not check if notification icon exists: %w", err)
	}

	return expectedLocation, nil
}

func New(db *bbolt.DB, opts ...notifierOption) (*Notifier, error) {
	// Create the bucket if it doesn't exist
	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(osquery.SentNotificationsBucket))
		if err != nil {
			return fmt.Errorf("could not create bucket: %w", err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("cannot create notifier without db bucket: %w", err)
	}

	notifier := &Notifier{
		db:              db,
		logger:          log.NewNopLogger(),
		notificationTtl: time.Hour * 1,
	}

	for _, opt := range opts {
		opt(notifier)
	}

	return notifier, nil
}

func (n *Notifier) Notify(title, body, uuid string) {
	if n.notificationAlreadySent(title, body) {
		level.Debug(n.logger).Log("msg", "received duplicate notification", "title", title)
		return
	}

	n.sendNotification(title, body)

	n.markNotificationSent(sentNotification{
		Title:  title,
		Body:   body,
		UUID:   uuid,
		SentAt: time.Now(),
	})
}

func (n *Notifier) notificationAlreadySent(title, body string) bool {
	alreadySent := false

	if err := n.db.View(func(tx *bbolt.Tx) error {
		sentNotificationRaw := tx.Bucket([]byte(osquery.SentNotificationsBucket)).Get(notificationKey(title, body))
		if sentNotificationRaw == nil {
			// No previous record -- notification has not been sent before
			return nil
		}

		var previouslySentNotification sentNotification
		if err := json.Unmarshal(sentNotificationRaw, &previouslySentNotification); err != nil {
			return fmt.Errorf("could not unmarshal previously sent notification: %w", err)
		}

		// Check to see how long ago the previously-sent notification was sent -- if it's within the
		// configured TTL, then we consider it a duplicate and will not re-send.
		sentExpiredAt := previouslySentNotification.SentAt.Add(n.notificationTtl)
		alreadySent = sentExpiredAt.After(time.Now())

		return nil
	}); err != nil {
		level.Debug(n.logger).Log("msg", "could not read sent notifications from bucket", "err", err)
	}

	return alreadySent
}

func (n *Notifier) markNotificationSent(s sentNotification) {
	if err := n.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(osquery.SentNotificationsBucket))
		if err != nil {
			return fmt.Errorf("could not create bucket: %w", err)
		}

		rawNotification, err := json.Marshal(s)
		if err != nil {
			return fmt.Errorf("could not marshal sent notification: %w", err)
		}

		if err := b.Put(notificationKey(s.Title, s.Body), rawNotification); err != nil {
			return fmt.Errorf("could not write to key: %w", err)
		}

		return nil
	}); err != nil {
		level.Error(n.logger).Log("msg", "could not mark notification sent", "title", s.Title, "err", err)
	}
}

func notificationKey(title string, body string) []byte {
	combined := fmt.Sprintf("%d:%s #### %d:%s", len(title), title, len(body), body)
	h := sha256.New()
	h.Write([]byte(combined))
	key := fmt.Sprintf("%x", h.Sum(nil))
	return []byte(key)
}
