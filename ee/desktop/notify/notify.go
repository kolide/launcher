package notify

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	Title string
	Body  string
	Sent  time.Time
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

func New(db *bbolt.DB, opts ...notifierOption) *Notifier {
	notifier := &Notifier{
		db:              db,
		logger:          log.NewNopLogger(),
		notificationTtl: time.Hour * 1,
	}

	for _, opt := range opts {
		opt(notifier)
	}

	return notifier
}

func (n *Notifier) Notify(title, body string) {
	if n.notificationAlreadySent(title, body) {
		level.Debug(n.logger).Log("msg", "received duplicate notification", "title", title)
		return
	}

	n.sendNotification(title, body)

	s := sentNotification{
		Title: title,
		Body:  body,
		Sent:  time.Now(),
	}

	n.markNotificationSent(s)
}

func (n *Notifier) notificationAlreadySent(title, body string) bool {
	sent := false
	if err := n.db.View(func(tx *bbolt.Tx) error {
		sentTimestampStr := tx.Bucket([]byte(osquery.SentNotificationsBucket)).Get(notificationKey(title, body))
		if sentTimestampStr == nil {
			return nil
		}

		sentTimestampInt, err := strconv.ParseInt(string(sentTimestampStr), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid timestamp %s: %w", sentTimestampStr, err)
		}

		sentTime := time.Unix(sentTimestampInt, 0)
		sentExpiredAt := sentTime.Add(n.notificationTtl)
		sent = sentExpiredAt.After(time.Now())

		return nil
	}); err != nil {
		level.Debug(n.logger).Log("msg", "could not read sent notifications from bucket", "err", err)
	}

	return sent
}

func (n *Notifier) markNotificationSent(s sentNotification) error {
	if err := n.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(osquery.SentNotificationsBucket))
		if err != nil {
			return fmt.Errorf("could not create bucket: %w", err)
		}

		if err := b.Put(notificationKey(s.Title, s.Body), s.notificationValue()); err != nil {
			return fmt.Errorf("could not write to key: %w", err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("could not mark notification sent: %w", err)
	}

	return nil
}

func notificationKey(title string, body string) []byte {
	combined := fmt.Sprintf("%d:%s #### %d:%s", len(title), title, len(body), body)
	h := sha256.New()
	h.Write([]byte(combined))
	key := fmt.Sprintf("%x", h.Sum(nil))
	return []byte(key)
}

func (s *sentNotification) notificationValue() []byte {
	timeStr := strconv.Itoa(int(s.Sent.Unix()))
	return []byte(timeStr)
}
