//go:build linux
// +build linux

package notify

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/godbus/dbus/v5"
)

type dbusNotifier struct {
	iconFilepath        string
	logger              log.Logger
	conn                *dbus.Conn
	signal              chan *dbus.Signal
	interrupt           chan struct{}
	sentNotificationIds map[uint32]bool
	lock                sync.RWMutex
}

const (
	notificationServiceObj       = "/org/freedesktop/Notifications"
	notificationServiceInterface = "org.freedesktop.Notifications"
	signalActionInvoked          = "org.freedesktop.Notifications.ActionInvoked"
)

func newOsSpecificNotifier(logger log.Logger, iconFilepath string) *dbusNotifier {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		level.Warn(logger).Log("msg", "couldn't connect to dbus to start notifier listener, proceeding without it", "err", err)
	}

	return &dbusNotifier{
		iconFilepath:        iconFilepath,
		logger:              logger,
		conn:                conn,
		signal:              make(chan *dbus.Signal, 10),
		interrupt:           make(chan struct{}),
		sentNotificationIds: make(map[uint32]bool),
		lock:                sync.RWMutex{},
	}
}

func (d *dbusNotifier) Listen() error {
	if d.conn != nil {
		if err := d.conn.AddMatchSignal(
			dbus.WithMatchObjectPath(notificationServiceObj),
			dbus.WithMatchInterface(notificationServiceInterface),
		); err != nil {
			level.Error(d.logger).Log("msg", "couldn't add match signal", "err", err)
			return fmt.Errorf("couldn't register to listen to signals in dbus: %w", err)
		}
		d.conn.Signal(d.signal)
	} else {
		level.Warn(d.logger).Log("msg", "cannot set up DBUS listener -- no connection to session bus")
	}

	for {
		select {
		case signal := <-d.signal:
			if signal == nil || signal.Name != signalActionInvoked {
				continue
			}

			// Confirm that this is a Kolide-originated notification by checking for known notification IDs
			notificationId := signal.Body[0].(uint32)
			d.lock.RLock()
			if _, found := d.sentNotificationIds[notificationId]; !found {
				// This notification didn't come from us -- ignore it
				d.lock.RUnlock()
				continue
			}
			d.lock.RUnlock()

			// Confirm that the URI is legitimate
			actionUri := signal.Body[1].(string)
			_, err := url.Parse(actionUri)
			if err != nil {
				level.Error(d.logger).Log("msg", "received invalid action URI from dbus", "action_uri", actionUri, "parse_err", err)
				continue
			}

			// Attempt to open a browser to the given URL
			providers := []string{"/usr/bin/xdg-open", "/usr/bin/x-www-browser"}
			for _, provider := range providers {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				cmd := exec.CommandContext(ctx, provider, actionUri)
				cmd.Env = append(cmd.Env, "DISPLAY=:10.0") // TODO RM - this is an xrdp workaround
				if err := cmd.Start(); err != nil {
					level.Error(d.logger).Log("msg", "couldn't start process", "err", err, "provider", provider)
				} else {
					break
				}
			}

		case <-d.interrupt:
			return nil
		}
	}
}

func (d *dbusNotifier) Interrupt(err error) {
	d.interrupt <- struct{}{}

	d.conn.RemoveSignal(d.signal)
	d.conn.RemoveMatchSignal(
		dbus.WithMatchObjectPath(notificationServiceObj),
		dbus.WithMatchInterface(notificationServiceInterface),
	)
}

func (d *dbusNotifier) SendNotification(title, body, actionUri string) error {
	if err := d.sendNotificationViaDbus(title, body, actionUri); err == nil {
		return nil
	}

	return d.sendNotificationViaNotifySend(title, body, actionUri)
}

// See: https://specifications.freedesktop.org/notification-spec/notification-spec-latest.html
func (d *dbusNotifier) sendNotificationViaDbus(title, body, actionUri string) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		level.Debug(d.logger).Log("msg", "could not connect to dbus, will try alternate method of notification", "err", err)
		return fmt.Errorf("could not connect to dbus: %w", err)
	}

	actions := []string{}
	if actionUri != "" {
		actions = append(actions, actionUri, "Open")
	}

	notificationsService := conn.Object(notificationServiceInterface, notificationServiceObj)
	call := notificationsService.Call("org.freedesktop.Notifications.Notify",
		0,                         // no flags
		"Kolide",                  // app_name
		uint32(0),                 // replaces_id -- 0 means this notification won't replace any existing notifications
		d.iconFilepath,            // app_icon
		title,                     // summary
		body,                      // body
		actions,                   // actions
		map[string]dbus.Variant{}, // hints
		int32(0))                  // expire_timeout -- 0 means the notification will not expire

	if call.Err != nil {
		level.Error(d.logger).Log("msg", "could not send notification via dbus", "err", call.Err)
		return fmt.Errorf("could not send notification via dbus: %w", call.Err)
	}

	// Save the notification ID from the response
	var notificationId uint32
	if err := call.Store(&notificationId); err != nil {
		level.Warn(d.logger).Log("msg", "could not get notification ID from dbus call", "err", err)
	} else {
		d.lock.Lock()
		defer d.lock.Unlock()
		d.sentNotificationIds[notificationId] = true
	}

	return nil
}

func (d *dbusNotifier) sendNotificationViaNotifySend(title, body, actionUri string) error {
	notifySend, err := exec.LookPath("notify-send")
	if err != nil {
		level.Debug(d.logger).Log("msg", "notify-send not installed", "err", err)
		return fmt.Errorf("notify-send not installed: %w", err)
	}

	// notify-send doesn't support actions, but URLs in notifications are clickable.
	if actionUri != "" {
		body += " Open: " + actionUri
	}

	args := []string{title, body}
	if d.iconFilepath != "" {
		args = append(args, "-i", d.iconFilepath)
	}

	cmd := exec.Command(notifySend, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		level.Error(d.logger).Log("msg", "could not send notification via notify-send", "output", string(out), "err", err)
		return fmt.Errorf("could not send notification via notify-send: %s: %w", string(out), err)
	}

	return nil
}
