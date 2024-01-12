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

	var usersToCheck []*user.User

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("getting current user: %w", err)
	}

	usersToCheck = append(usersToCheck, currentUser)

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

// func execGnomeExtension(ctx context.Context, extraWriter io.Writer, rundir string, args ...string) ([]byte, error) {
// 	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
// 	defer cancel()

// 	// TODO: Need to figure out how to make this run per user
// 	// ee/tables/gsettings/gsettings.go probably has appropriate prior art.
// 	// But do we really want the forloop?

// 	consoleUsers, err := consoleuser.CurrentUsers(ctx)
// 	if err != nil {
// 		return nil, fmt.Errorf("getting current users: %w", err)
// 	}

// 	for _, usr := range consoleUsers {
// 		cmd, err := allowedcmd.GnomeExtensions(ctx, args...)
// 		if err != nil {
// 			return nil, fmt.Errorf("creating gnome-extensions command: %w", err)
// 		}

// 		// gnome seems to do things through this env
// 		cmd.Env = append(cmd.Env, fmt.Sprintf("XDG_RUNTIME_DIR=/run/user/%s", usr.Uid))

// 		buf := &bytes.Buffer{}
// 		cmd.Stderr = io.MultiWriter(extraWriter, buf)
// 		cmd.Stdout = cmd.Stderr

// 		// A bit of an experiment in output formatting. Make it look like a markdown command block
// 		fmt.Fprintf(extraWriter, "```\n$ %s\n", cmd.String())
// 		defer fmt.Fprintf(extraWriter, "```\n\n")

// 		if err := cmd.Run(); err != nil {
// 			// reset the buffer so we don't return the error code
// 			return nil, fmt.Errorf(`running "%s", err is: %s: %w`, cmd.String(), buf.String(), err)
// 		}
// 	}

// 	return buf.Bytes(), nil
// }

// func checkRundir(ctx context.Context, extraWriter io.Writer, rundir string) (Status, string) {
// 	fmt.Fprintf(extraWriter, "## Checking rundir %s\n\n", rundir)

// 	status := Unknown
// 	summary := "unknown"

// 	missing := []string{}

// 	for _, ext := range expectedExtensions {
// 		fmt.Fprintf(extraWriter, "### %s\n\n", ext)

// 		output, err := execGnomeExtension(ctx, extraWriter, rundir, "show", ext)
// 		if err != nil {
// 			// Errors running this command are probably fatal, may as well bail
// 			return Erroring, fmt.Sprintf("error running gnome-extensions: %s", err)
// 		}

// 		// Is it enabled?
// 		if !bytes.Contains(output, []byte("State: ENABLED")) {
// 			missing = append(missing, ext)
// 		}
// 	}

// 	if len(missing) > 0 {
// 		status = Failing
// 		summary = fmt.Sprintf("missing (or screenlocked) extensions: %s", strings.Join(missing, ", "))
// 	} else {
// 		status = Passing
// 		summary = fmt.Sprintf("enabled extensions: %s", strings.Join(expectedExtensions, ", "))
// 	}

// 	if extraWriter != io.Discard {
// 		// We can ignore the response, because it's tee'ed into extraWriter
// 		_, _ = execGnomeExtension(ctx, extraWriter, rundir, "list")

// 	}

// 	return status, summary

// }
