//go:build linux
// +build linux

package notify

import (
	"github.com/go-kit/kit/log/level"
	"github.com/ncruces/zenity"
)

func (n *Notifier) sendNotification(title, body string) {
	if err := zenity.Notify(body, zenity.Title(title), zenity.InfoIcon); err != nil {
		level.Error(n.logger).Log("msg", "could not send notification", "title", title, "err", err)
	}
}
