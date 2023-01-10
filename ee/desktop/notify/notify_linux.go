//go:build linux
// +build linux

package notify

import (
	"fmt"
	"os/exec"

	"github.com/go-kit/kit/log/level"
	"github.com/godbus/dbus/v5"
)

func (d *DesktopNotifier) SendNotification(title, body string) error {
	if err := d.sendNotificationViaDbus(title, body); err == nil {
		return nil
	}

	return d.sendNotificationViaNotifySend(title, body)
}

// See: https://specifications.freedesktop.org/notification-spec/notification-spec-latest.html
func (d *DesktopNotifier) sendNotificationViaDbus(title, body string) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		level.Debug(d.logger).Log("msg", "could not connect to dbus, will try alternate method of notification", "err", err)
		return fmt.Errorf("could not connect to dbus: %w", err)
	}

	notificationsService := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	call := notificationsService.Call("org.freedesktop.Notifications.Notify",
		0,                         // no flags
		"Kolide",                  // app_name
		uint32(0),                 // replaces_id -- 0 means this notification won't replace any existing notifications
		d.iconFilepath,            // app_icon
		title,                     // summary
		body,                      // body
		[]string{},                // actions
		map[string]dbus.Variant{}, // hints
		int32(0))                  // expire_timeout -- 0 means the notification will not expire

	if call.Err != nil {
		level.Error(d.logger).Log("msg", "could not send notification via dbus", "err", call.Err)
		return fmt.Errorf("could not send notification via dbus: %w", call.Err)
	}

	return nil
}

func (d *DesktopNotifier) sendNotificationViaNotifySend(title, body string) error {
	notifySend, err := exec.LookPath("notify-send")
	if err != nil {
		level.Debug(d.logger).Log("msg", "notify-send not installed", "err", err)
		return fmt.Errorf("notify-send not installed: %w", err)
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
