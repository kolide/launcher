package notify

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/ee/desktop/assets"
)

type notifier interface {
	sendNotification(title, body string) error
}

type desktopNotifier struct {
	logger       log.Logger
	iconFilepath string
}

func newDesktopNotifier(logger log.Logger, rootDir string) *desktopNotifier {
	notifier := &desktopNotifier{
		logger: logger,
	}

	iconPath, err := setIconPath(rootDir)
	if err != nil {
		level.Error(notifier.logger).Log("msg", "could not set icon path for notifications", "err", err)
	} else {
		notifier.iconFilepath = iconPath
	}

	return notifier
}

func setIconPath(rootDir string) (string, error) {
	expectedLocation := filepath.Join(rootDir, assets.KolideIconFilename)

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
