//go:build linux
// +build linux

package notify

import (
	"github.com/go-kit/kit/log/level"
)

func (n *Notifier) sendNotification(title, body string) {
	level.Error(n.logger).Log("msg", "notifications not yet implemented for linux, sorry", "title", title)
}
