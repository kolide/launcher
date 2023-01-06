//go:build linux
// +build linux

package notify

import (
	"fmt"
	"os/exec"

	"github.com/go-kit/kit/log/level"
	"github.com/godbus/dbus/v5"
)

func (n *Notifier) sendNotification(title, body string) error {
	if err := n.sendNotificationViaDbus(title, body); err == nil {
		return nil
	}

	return n.sendNotificationViaNotifySend(title, body)
}

// See: https://specifications.freedesktop.org/notification-spec/notification-spec-latest.html
func (n *Notifier) sendNotificationViaDbus(title, body string) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("could not connect to dbus: %w", err)
	}

	notificationsService := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	call := notificationsService.Call("org.freedesktop.Notifications.Notify",
		0,                         // no flags
		"Kolide",                  // app_name
		uint32(0),                 // replaces_id -- 0 means this notification won't replace any existing notifications
		n.iconFilepath,            // app_icon
		title,                     // summary
		body,                      // body
		[]string{},                // actions
		map[string]dbus.Variant{}, // hints
		int32(0))                  // expire_timeout -- 0 means the notification will not expire

	if call.Err != nil {
		level.Error(n.logger).Log("msg", "could not send notification via dbus", "err", call.Err)
		return fmt.Errorf("could not send notification via dbus: %w", call.Err)
	}

	return nil
}

func (n *Notifier) sendNotificationViaNotifySend(title, body string) error {
	notifySend, err := exec.LookPath("notify-send")
	if err != nil {
		return fmt.Errorf("notify-send not installed: %w", err)
	}

	args := []string{title, body}
	if n.iconFilepath != "" {
		args = append(args, "-i", n.iconFilepath)
	}

	cmd := exec.Command(notifySend, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		level.Error(n.logger).Log("msg", "could not send notification via notify-send", "output", string(out), "err", err)
		return fmt.Errorf("could not send notification via notify-send: %w", err)
	}

	return nil
}
