package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/startupsettings"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/multislogger"
)

func runRecheck(_ *multislogger.MultiSlogger, _ []string) error {
	attachConsole()
	defer detachConsole()

	settingReader, err := startupsettings.OpenReader(context.TODO(), launcher.DefaultPath(launcher.RootDirectory))
	if err != nil {
		return fmt.Errorf("opening startup settings reader to fetch desktop runner server url, is launcher daemon running?: %w", err)
	}
	defer settingReader.Close()

	desktopRunnerServerURL, err := settingReader.Get(keys.DesktopRunnerServerUrl.String())
	if err != nil {
		return fmt.Errorf("getting desktop runner server url, is launcher daemon running?: %w", err)
	}

	response, err := http.Get(fmt.Sprintf("%s/recheck", desktopRunnerServerURL))
	if err != nil {
		return fmt.Errorf("sending recheck request to desktop runner server, is launcher daemon running?: %w", err)
	}

	if response.Body != nil {
		defer response.Body.Close()
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("recheck request failed with status code %d", response.StatusCode)
	}

	fmt.Print("recheck request sent successfully")

	return nil
}
