//go:build linux
// +build linux

package menu

import (
	"fmt"
	"os/exec"
)

var browserLaunchers = []string{"xdg-open", "x-www-browser"}

func open(url string) error {
	errList := make([]error, 0)
	for _, browserLauncher := range browserLaunchers {
		cmd := exec.Command(browserLauncher, url)
		if err := cmd.Start(); err != nil {
			errList = append(errList, fmt.Errorf("could not open browser with %s: %w", browserLauncher, err))
			continue
		}

		// Successfully opened URL, nothing else to do here
		return nil
	}
	return fmt.Errorf("could not successfully open browser with any given launchers: %+v", errList)
}
