//go:build linux
// +build linux

package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/godbus/dbus/v5"
	"github.com/kolide/launcher/ee/allowedcmd"
)

type dbusNotifier struct {
	iconFilepath        string
	slogger             *slog.Logger
	conn                *dbus.Conn
	signal              chan *dbus.Signal
	interrupt           chan struct{}
	interrupted         atomic.Bool
	sentNotificationIds map[uint32]bool
	lock                sync.RWMutex
}

const (
	notificationServiceObj       = "/org/freedesktop/Notifications"
	notificationServiceInterface = "org.freedesktop.Notifications"
	signalActionInvoked          = "org.freedesktop.Notifications.ActionInvoked"
)

// We default to xdg-open first because, if available, it appears to be better at picking
// the correct default browser.
var browserLaunchers = []allowedcmd.AllowedCommand{allowedcmd.XdgOpen, allowedcmd.XWwwBrowser}

func NewDesktopNotifier(slogger *slog.Logger, iconFilepath string) *dbusNotifier {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		slogger.Log(context.TODO(), slog.LevelWarn,
			"couldn't connect to dbus to start notifier listener, proceeding without it",
			"err", err,
		)
	}

	return &dbusNotifier{
		iconFilepath:        iconFilepath,
		slogger:             slogger.With("component", "desktop_notifier"),
		conn:                conn,
		signal:              make(chan *dbus.Signal),
		interrupt:           make(chan struct{}),
		sentNotificationIds: make(map[uint32]bool),
		lock:                sync.RWMutex{},
	}
}

func (d *dbusNotifier) Execute() error {
	if d.conn != nil {
		if err := d.conn.AddMatchSignal(
			dbus.WithMatchObjectPath(notificationServiceObj),
			dbus.WithMatchInterface(notificationServiceInterface),
		); err != nil {
			d.slogger.Log(context.TODO(), slog.LevelError,
				"couldn't add match signal",
				"err", err,
			)
			return fmt.Errorf("couldn't register to listen to signals in dbus: %w", err)
		}
		d.conn.Signal(d.signal)
	} else {
		d.slogger.Log(context.TODO(), slog.LevelWarn,
			"cannot set up DBUS listener -- no connection to session bus",
		)
	}

	for {
		select {
		case signal, open := <-d.signal:
			if !open {
				return errors.New("dbus signal channel closed, cannot proceed")
			}

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

			// Attempt to open a browser to the given URL
			actionUri := signal.Body[1].(string)

			for _, browserLauncher := range browserLaunchers {
				cmd, err := browserLauncher(context.TODO(), actionUri)
				if err != nil {
					d.slogger.Log(context.TODO(), slog.LevelWarn,
						"couldn't create command to start process",
						"err", err,
						"browser_launcher", browserLauncher,
					)
					continue
				}

				err = cmd.Start()
				if err == nil {
					break
				}
				d.slogger.Log(context.TODO(), slog.LevelError,
					"couldn't start process",
					"err", err,
					"browser_launcher", browserLauncher,
				)
			}

		case <-d.interrupt:
			return nil
		}
	}
}

func (d *dbusNotifier) Interrupt(err error) {
	if d.interrupted.Swap(true) {
		return
	}

	d.interrupt <- struct{}{}

	d.conn.RemoveSignal(d.signal)
	d.conn.RemoveMatchSignal(
		dbus.WithMatchObjectPath(notificationServiceObj),
		dbus.WithMatchInterface(notificationServiceInterface),
	)
}

// just make compiler happy, this is only needed on darwin
func (d *dbusNotifier) Listen() {}

func (d *dbusNotifier) SendNotification(n Notification) error {
	if err := d.sendNotificationViaDbus(n); err == nil {
		return nil
	}

	return d.sendNotificationViaNotifySend(n)
}

// See: https://specifications.freedesktop.org/notification-spec/notification-spec-latest.html
func (d *dbusNotifier) sendNotificationViaDbus(n Notification) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		d.slogger.Log(context.TODO(), slog.LevelDebug,
			"could not connect to dbus, will try alternate method of notification",
			"err", err,
		)
		return fmt.Errorf("could not connect to dbus: %w", err)
	}

	actions := []string{}
	if n.ActionUri != "" {
		actions = append(actions, n.ActionUri, "Learn More")
	}

	notificationsService := conn.Object(notificationServiceInterface, notificationServiceObj)
	call := notificationsService.Call("org.freedesktop.Notifications.Notify",
		0,                         // no flags
		"Kolide",                  // app_name
		uint32(0),                 // replaces_id -- 0 means this notification won't replace any existing notifications
		d.iconFilepath,            // app_icon
		n.Title,                   // summary
		n.Body,                    // body
		actions,                   // actions
		map[string]dbus.Variant{}, // hints
		int32(0))                  // expire_timeout -- 0 means the notification will not expire

	if call.Err != nil {
		d.slogger.Log(context.TODO(), slog.LevelError,
			"could not send notification via dbus",
			"err", call.Err,
		)
		return fmt.Errorf("could not send notification via dbus: %w", call.Err)
	}

	// Save the notification ID from the response -- notifications don't appear to persist on reboot
	// so we should be fine to only store these here.
	var notificationId uint32
	if err := call.Store(&notificationId); err != nil {
		d.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not get notification ID from dbus call",
			"err", err,
		)
	} else {
		d.lock.Lock()
		defer d.lock.Unlock()
		d.sentNotificationIds[notificationId] = true
	}

	return nil
}

func (d *dbusNotifier) sendNotificationViaNotifySend(n Notification) error {
	// notify-send doesn't support actions, but URLs in notifications are clickable in at least
	// some desktop environments.
	if n.ActionUri != "" {
		n.Body += " Learn More: " + n.ActionUri
	}

	args := []string{n.Title, n.Body}
	if d.iconFilepath != "" {
		args = append(args, "-i", d.iconFilepath)
	}

	cmd, err := allowedcmd.NotifySend(context.TODO(), args...)
	if err != nil {
		return fmt.Errorf("creating command: %w", err)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		d.slogger.Log(context.TODO(), slog.LevelError,
			"could not send notification via notify-send",
			"output", string(out),
			"err", err,
		)
		return fmt.Errorf("could not send notification via notify-send: %s: %w", string(out), err)
	}

	return nil
}
