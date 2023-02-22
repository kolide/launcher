//go:build windows
// +build windows

package notify

import (
	"github.com/go-kit/kit/log"
	"gopkg.in/toast.v1"
)

type windowsNotifier struct {
	iconFilepath string
	logger       log.Logger
	interrupt    chan struct{}
}

func newOsSpecificNotifier(logger log.Logger, iconFilepath string) *windowsNotifier {
	return &windowsNotifier{
		iconFilepath: iconFilepath,
		logger:       logger,
		interrupt:    make(chan struct{}),
	}
}

// Listen doesn't do anything on Windows -- the `launch` variable in the notification XML
// automatically handles opening URLs for us.
func (w *windowsNotifier) Listen() error {
	for {
		select {
		case <-w.interrupt:
			return nil
		}
	}
}

func (w *windowsNotifier) Interrupt(err error) {
	w.interrupt <- struct{}{}
}

func (w *windowsNotifier) SendNotification(title, body, actionUri string) error {
	notification := toast.Notification{
		AppID:   "Kolide",
		Title:   title,
		Message: body,
	}

	if w.iconFilepath != "" {
		notification.Icon = w.iconFilepath
	}

	if actionUri != "" {
		// Set the default action when the user clicks on the notification
		notification.ActivationArguments = actionUri

		// Additionally, create a "Learn more" button that will open the same URL
		notification.Actions = []toast.Action{
			{
				Type:      "protocol",
				Label:     "Learn more",
				Arguments: actionUri,
			},
		}
	}

	return notification.Push()
}
