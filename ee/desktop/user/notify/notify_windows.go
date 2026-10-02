//go:build windows

package notify

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"

	kolideatomic "github.com/kolide/launcher/v2/pkg/atomic"
	"github.com/kolide/toast"
)

type windowsNotifier struct {
	slogger          *slog.Logger
	iconFilepath     *kolideatomic.String
	localizationPath string
	interrupt        chan struct{}
	interrupted      atomic.Bool
}

func NewDesktopNotifier(slogger *slog.Logger, iconFilepath string, localizationPath string) *windowsNotifier {
	return &windowsNotifier{
		slogger:          slogger.With("component", "desktop_notifier"),
		iconFilepath:     kolideatomic.NewString(iconFilepath),
		localizationPath: localizationPath,
		interrupt:        make(chan struct{}),
	}
}

// Listen doesn't do anything on Windows -- the `launch` variable in the notification XML
// automatically handles opening URLs for us.
func (w *windowsNotifier) Execute() error {
	<-w.interrupt
	return nil
}

// just make compiler happy, this is only needed on darwin
func (w *windowsNotifier) Listen() {}

// SetIconFilepath updates the icon used for subsequent notifications
func (w *windowsNotifier) SetIconFilepath(iconFilepath string) {
	w.iconFilepath.Store(iconFilepath)
}

func (w *windowsNotifier) Interrupt(err error) {
	if w.interrupted.Swap(true) {
		return
	}

	w.interrupt <- struct{}{}
}

func (w *windowsNotifier) SendNotification(n Notification) error {
	notification := toast.Notification{
		AppID:   "Kolide",
		Title:   n.Title,
		Message: n.Body,
	}

	if iconFilepath := w.iconFilepath.Load(); iconFilepath != "" {
		notification.Icon = iconFilepath
	}

	if n.ActionUri != "" {
		actionUri := strings.ReplaceAll(n.ActionUri, "&", "&amp;")

		// Set the default action when the user clicks on the notification
		notification.ActivationArguments = actionUri

		// Additionally, create a "Learn more" button that will open the same URL
		notification.Actions = []toast.Action{
			{
				Type:      "protocol",
				Label:     learnMoreLabel(w.localizationPath),
				Arguments: actionUri,
			},
		}
	}

	if err := notification.Push(); err != nil {
		w.slogger.Log(context.TODO(), slog.LevelError,
			"could not send toast notification",
			"title", n.Title,
			"err", err,
		)
		return fmt.Errorf("sending toast notification: %w", err)
	}

	return nil
}
