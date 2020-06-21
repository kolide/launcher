package cmdwrapper

import (
	"os/exec"
	"os/user"
	"strconv"
	"syscall"

	"github.com/pkg/errors"
)

func (eo *execOptions) setRunAsUser(cmd *exec.Cmd) error {
	// Nothing to do, fast return
	if eo.runAsUser == "" {
		return nil
	}

	currentUser, err := user.Current()
	if err != nil {
		return errors.Wrap(err, "determining current user")
	}

	// If the requested user, matches current, early return
	if eo.runAsUser == currentUser.Username && !eo.skipUserCheck {
		return nil
	}

	targetUser, err := user.Lookup(eo.runAsUser)
	if err != nil {
		return errors.Wrapf(err, "looking up username %s", eo.runAsUser)
	}

	targetUid, err := strconv.Atoi(targetUser.Uid)
	if err != nil {
		return errors.Wrapf(err, "converting user uid %s to int", targetUser.Uid)
	}

	targetGid, err := strconv.Atoi(targetUser.Gid)
	if err != nil {
		return errors.Wrapf(err, "converting user gid %s to int", targetUser.Gid)
	}

	targetPermissions := &syscall.Credential{
		Uid: uint32(targetUid),
		Gid: uint32(targetGid),
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: targetPermissions,
	}

	return nil

}
