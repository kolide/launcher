//go:build windows
// +build windows

package notify

import (
	"github.com/go-kit/kit/log/level"
	"gopkg.in/toast.v1"
)

func (n *Notifier) sendNotification(title, body string) {
	notification := toast.Notification{
		AppID:   "Kolide",
		Title:   title,
		Message: body,
		Actions: []toast.Action{},
	}

	if n.iconFilepath != "" {
		notification.Icon = n.iconFilepath
	}

	if err := notification.Push(); err != nil {
		level.Error(n.logger).Log("msg", "could not send toast", "title", title, "err", err)
	}
}
