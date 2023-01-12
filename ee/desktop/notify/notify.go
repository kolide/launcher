package notify

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/ee/desktop/assets"
)

type DesktopNotifier struct {
	logger       log.Logger
	iconFilepath string
}

type Notification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

func NewDesktopNotifier(logger log.Logger, iconDir string) *DesktopNotifier {
	notifier := &DesktopNotifier{
		logger: log.With(logger,
			"component", "user_desktop_notifier",
		),
	}

	iconPath, err := setIconPath(iconDir)
	if err != nil {
		level.Error(notifier.logger).Log("msg", "could not set icon path for notifications", "err", err)
	} else {
		notifier.iconFilepath = iconPath
	}

	return notifier
}

func setIconPath(iconDir string) (string, error) {
	expectedLocation := filepath.Join(iconDir, assets.KolideIconFilename)

	_, err := os.Stat(expectedLocation)

	if os.IsNotExist(err) {
		if err := os.WriteFile(expectedLocation, assets.KolideDesktopIcon, 0644); err != nil {
			return "", fmt.Errorf("notification icon did not exist; could not create it: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("could not check if notification icon exists: %w", err)
	}

	return expectedLocation, nil
}
