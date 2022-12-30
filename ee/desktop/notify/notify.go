package notify

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"fyne.io/fyne"
	"fyne.io/fyne/app"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

type Notifier struct {
	sentNotifications map[string]Notification // for now, just track sent notifications in memory
	lock              *sync.RWMutex
	logger            log.Logger
	notificationTtl   time.Duration
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

	// We create a fyne app to be able to send the notification. The notification process doesn't
	// actually require the app to be running.
	tempApp := app.NewWithID("com.kolide.desktop")
	tempApp.SendNotification(fyne.NewNotification(title, body))

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
