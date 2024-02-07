//go:build windows
// +build windows

package notify

import (
	"log/slog"

	"github.com/kolide/toast"
)

type windowsNotifier struct {
	iconFilepath string
	interrupt    chan struct{}
}

func NewDesktopNotifier(_ *slog.Logger, iconFilepath string) *windowsNotifier {
	return &windowsNotifier{
		iconFilepath: iconFilepath,
		interrupt:    make(chan struct{}),
	}
}

// Listen doesn't do anything on Windows -- the `launch` variable in the notification XML
// automatically handles opening URLs for us.
func (w *windowsNotifier) Listen() error {
	<-w.interrupt
	return nil
}

func (w *windowsNotifier) Interrupt(err error) {
	w.interrupt <- struct{}{}
}

func (w *windowsNotifier) SendNotification(n Notification) error {
	notification := toast.Notification{
		AppID:   "Kolide",
		Title:   n.Title,
		Message: n.Body,
	}

	if w.iconFilepath != "" {
		notification.Icon = w.iconFilepath
	}

	if n.ActionUri != "" {
		// Set the default action when the user clicks on the notification
		notification.ActivationArguments = n.ActionUri

		// Additionally, create a "Learn more" button that will open the same URL
		notification.Actions = []toast.Action{
			{
				Type:      "protocol",
				Label:     "Learn More",
				Arguments: n.ActionUri,
			},
		}
	}

	return notification.Push()
}
