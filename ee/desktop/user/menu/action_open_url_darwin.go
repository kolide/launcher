//go:build darwin
// +build darwin

package menu

import (
	"context"
	"fmt"

	"github.com/kolide/launcher/pkg/allowedpaths"
)

// open opens the specified URL in the default browser of the user
// See https://stackoverflow.com/a/39324149/1705598
func open(url string) error {
	cmd, err := allowedpaths.Open(context.TODO(), url)
	if err != nil {
		return fmt.Errorf("creating command: %w", err)
	}

	return cmd.Start()
}
