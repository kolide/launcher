//go:build linux
// +build linux

package checkups

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/user"
	"strconv"
	"syscall"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/consoleuser"
)

type gnomeExtensions struct {
	status  Status
	summary string
}

var expectedExtensions = []string{
	"ubuntu-appindicators@ubuntu.com",
}

func (c *gnomeExtensions) Name() string {
	return "Gnome Extensions"
}

func (c *gnomeExtensions) ExtraFileName() string {
	return "extensions.log"
}

func (c *gnomeExtensions) Run(ctx context.Context, extraWriter io.Writer) error {
	fmt.Fprintf(extraWriter, "# Checking Gnome Extensions\n\n")

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("getting current user: %w", err)
	}

	usersToCheck := []*user.User{currentUser}

	// running as root so we need to get the console users
	if currentUser.Uid == "0" {
		// replace users to check with console users
		usersToCheck, err = consoleuser.CurrentUsers(ctx)
		if err != nil {
			return fmt.Errorf("getting console users: %w", err)
		}
	}

	atLeastOneGnomeUser := false
	atleastOneUserHasExtensions := false

	for _, consoleUser := range usersToCheck {
		fmt.Fprintf(extraWriter, "## Checking user %s\n\n", consoleUser.Uid)

		cmd, err := allowedcmd.GnomeExtensions(ctx, "list", "--enabled")
		if err != nil {
			return fmt.Errorf("creating gnome-extensions list command: %w", err)
		}

		// if we are root, need to execute as the user
		if currentUser.Uid == "0" {
			runningUserUid, err := strconv.ParseUint(consoleUser.Uid, 10, 32)
			if err != nil {
				return fmt.Errorf("converting uid %s to int: %w", consoleUser.Uid, err)
			}

			runningUserGid, err := strconv.ParseUint(consoleUser.Gid, 10, 32)
			if err != nil {
				return fmt.Errorf("converting gid %s to int: %w", consoleUser.Gid, err)
			}

			cmd.SysProcAttr = &syscall.SysProcAttr{
				Credential: &syscall.Credential{
					Uid: uint32(runningUserUid),
					Gid: uint32(runningUserGid),
				},
			}

			cmd.Env = append(cmd.Env, fmt.Sprintf("XDG_RUNTIME_DIR=/run/user/%s", consoleUser.Uid))
		}

		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(extraWriter, "Error running gnome-extensions list, assuming user not using gnome, out: %s, err: %s\n\n", string(out), err)
			continue
		}

		atLeastOneGnomeUser = true

		thisUserHasAllExtensions := true

		for _, ext := range expectedExtensions {

			fmt.Fprintf(extraWriter, "### checking for extension %s\n\n", ext)

			if !bytes.Contains(out, []byte(ext)) {
				fmt.Fprintf(extraWriter, "User %s does not have extension %s enabled\n\n", consoleUser.Uid, ext)
				thisUserHasAllExtensions = false
				continue
			}

			fmt.Fprintf(extraWriter, "User %s has extension %s enabled\n\n", consoleUser.Uid, ext)
		}

		if thisUserHasAllExtensions {
			atleastOneUserHasExtensions = true
		}
	}

	if !atLeastOneGnomeUser {
		c.status = Unknown
		c.summary = "no gnome users found"
		return nil
	}

	if !atleastOneUserHasExtensions {
		c.status = Failing
		c.summary = "no user has all extensions enabled"
		return nil
	}

	c.status = Passing
	c.summary = "at least 1 user has all extensions enabled"
	return nil
}

func (c *gnomeExtensions) Status() Status {
	return c.status
}

func (c *gnomeExtensions) Summary() string {
	return c.summary
}

func (c *gnomeExtensions) Data() any {
	return nil
}
