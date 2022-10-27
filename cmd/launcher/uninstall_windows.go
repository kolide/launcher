//go:build windows
// +build windows

package main

import (
	"context"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
)

func removeLauncher(ctx context.Context, logger log.Logger, identifier string) error {
	// Launch the Windows Settings app using the ms-settings: URI scheme
	// https://learn.microsoft.com/en-us/windows/uwp/launch-resume/launch-settings-app#apps
	if _, err := tablehelpers.Exec(ctx, logger, 30, []string{"start"}, "ms-settings:appsfeatures"); err != nil {
		return err
	}

	return nil
}
