//go:build windows
// +build windows

package notify

import (
	"gopkg.in/toast.v1"
)

func (d *desktopNotifier) sendNotification(title, body string) error {
	notification := toast.Notification{
		AppID:   "Kolide",
		Title:   title,
		Message: body,
		Actions: []toast.Action{},
	}

	if d.iconFilepath != "" {
		notification.Icon = d.iconFilepath
	}

	return notification.Push()
}
