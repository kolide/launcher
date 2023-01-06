//go:build windows
// +build windows

package notify

import (
	"gopkg.in/toast.v1"
)

func (n *Notifier) sendNotification(title, body string) error {
	notification := toast.Notification{
		AppID:   "Kolide",
		Title:   title,
		Message: body,
		Actions: []toast.Action{},
	}

	if n.iconFilepath != "" {
		notification.Icon = n.iconFilepath
	}

	return notification.Push()
}
