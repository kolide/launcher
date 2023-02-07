package notify

import (
	"github.com/go-kit/kit/log"
)

type DesktopNotifier struct {
	logger       log.Logger
	iconFilepath string
}

type Notification struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	ActionUri string `json:"action_uri,omitempty"`
}

func NewDesktopNotifier(logger log.Logger, iconFilepath string) *DesktopNotifier {
	notifier := &DesktopNotifier{
		logger: log.With(logger,
			"component", "user_desktop_notifier",
		),
		iconFilepath: iconFilepath,
	}

	return notifier
}
