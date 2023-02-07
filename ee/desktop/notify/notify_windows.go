//go:build windows
// +build windows

package notify

import (
	"gopkg.in/toast.v1"
)

func (d *DesktopNotifier) SendNotification(title, body, actionUri string) error {
	notification := toast.Notification{
		AppID:   "Kolide",
		Title:   title,
		Message: body,
	}

	if d.iconFilepath != "" {
		notification.Icon = d.iconFilepath
	}

	if actionUri != "" {
		notification.ActivationArguments = actionUri
	}

	return notification.Push()
}
