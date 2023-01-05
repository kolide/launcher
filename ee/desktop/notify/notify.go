package notify

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/ee/desktop/assets"
)

type Notifier struct {
	sentNotifications map[string]Notification // for now, just track sent notifications in memory
	lock              *sync.RWMutex
	logger            log.Logger
	notificationTtl   time.Duration
	iconFilepath      string
}

type Notification struct {
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

func New(opts ...notifierOption) *Notifier {
	notifier := &Notifier{
		sentNotifications: make(map[string]Notification),
		lock:              &sync.RWMutex{},
		logger:            log.NewNopLogger(),
		notificationTtl:   time.Hour * 1,
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

	sentNotification := Notification{
		Title: title,
		Body:  body,
		Sent:  time.Now(),
	}

	n.markNotificationSent(sentNotification)
}

func (n *Notifier) notificationAlreadySent(title, body string) bool {
	n.lock.RLock()
	defer n.lock.RUnlock()

	k := notificationKey(title, body)
	existing, found := n.sentNotifications[k]
	if !found {
		return false
	}

	expiredAt := existing.Sent.Add(n.notificationTtl)
	return expiredAt.After(time.Now())
}

func (n *Notifier) markNotificationSent(sentNotification Notification) {
	n.lock.Lock()
	defer n.lock.Unlock()

	k := notificationKey(sentNotification.Title, sentNotification.Body)
	n.sentNotifications[k] = sentNotification
}

func notificationKey(title, body string) string {
	combined := fmt.Sprintf("%d:%s #### %d:%s", len(title), title, len(body), body)
	h := sha256.New()
	h.Write([]byte(combined))
	return fmt.Sprintf("%x", h.Sum(nil))
}
