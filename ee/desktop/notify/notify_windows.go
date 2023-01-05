//go:build windows
// +build windows

package notify

import (
	"os"
	"path/filepath"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/ee/desktop/assets"
	"gopkg.in/toast.v1"
)

func (n *Notifier) sendNotification(title, body string) {
	notification := toast.Notification{
		AppID:   "Kolide",
		Title:   title,
		Message: body,
		Actions: []toast.Action{},
	}

	if iconLocation := n.getIconLocation(); iconLocation != "" {
		notification.Icon = iconLocation
	}

	if err := notification.Push(); err != nil {
		level.Error(n.logger).Log("msg", "could not send toast", "title", title, "err", err)
	}
}

// We have to pass the API a file path, not the bytes, so create an icon file in our data directory.
func (n *Notifier) getIconLocation() string {
	expectedLocation := filepath.Join(n.dataDirectory, "kolide.ico")

	_, err := os.Stat(expectedLocation)

	// Create the file if it doesn't exist
	if os.IsNotExist(err) {
		if err := os.WriteFile(expectedLocation, assets.KolideDesktopIcon, 0644); err != nil {
			level.Error(n.logger).Log("msg", "icon did not exist; could not create it", "err", err)
			return ""
		}
	} else if err != nil {
		level.Error(n.logger).Log("msg", "could not check if icon exists", "err", err)
		return ""
	}

	return expectedLocation
}
