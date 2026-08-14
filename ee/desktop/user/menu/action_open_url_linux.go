//go:build linux

package menu

import (
	"context"
	"fmt"

	"github.com/kolide/launcher/v2/ee/allowedcmd"
	"github.com/kolide/launcher/v2/ee/desktop/user/notify"
)

// open opens the specified URL in the default browser of the user
// See https://stackoverflow.com/a/39324149/1705598
func open(url string) error {
	// Try via dbus before falling back to xdg-open --
	// we see improved behavior when using dbus.
	dbusErr := notify.OpenViaDbus(url)
	if dbusErr == nil {
		return nil
	}

	cmd, err := allowedcmd.XdgOpen.Cmd(context.TODO(), url)
	if err != nil {
		return fmt.Errorf("falling back creating command (after dbus failure %v): %w", dbusErr, err)
	}

	return cmd.Start()
}
