//go:build linux
// +build linux

package menu

import (
	"fmt"
	"os/exec"
)

// We default to x-www-browser first because, if available, it appears to be better at picking
// the correct default browser.
var browserLaunchers = []string{"x-www-browser", "xdg-open"}

func open(url string) error {
	errList := make([]error, 0)
	for _, browserLauncher := range browserLaunchers {
		cmd := exec.Command(browserLauncher, url)
		if err := cmd.Start(); err != nil {
			errList = append(errList, fmt.Errorf("could not open browser with %s: %w", browserLauncher, err))
		} else {
			// Successfully opened URL, nothing else to do here
			return nil
		}
	}
	return fmt.Errorf("could not successfully open browser with any given launchers: %+v", errList)
}
