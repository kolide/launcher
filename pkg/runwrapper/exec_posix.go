package runwrapper

import (
	"os/exec"
	"os/user"
	"strconv"
	"syscall"

	"github.com/pkg/errors"
)

func (eo *execOptions) setRunAs(cmd *exec.Cmd) error {
	// Nothing to do, fast return
	if eo.runAsUid == "" && eo.runAsGid == "" {
		return nil
	}

	currentUser, err := user.Current()
	if err != nil {
		return errors.Wrap(err, "determining current user")
	}

	myUid, err := strconv.Atoi(currentUser.Uid)
	if err != nil {
		return errors.Wrapf(err, "converting uid %s", currentUser.Uid)
	}

	myGid, err := strconv.Atoi(currentUser.Gid)
	if err != nil {
		return errors.Wrapf(err, "converting gid %s", currentUser.Gid)
	}

	targetPermissions := &syscall.Credential{
		Uid: uint32(myUid),
		Gid: uint32(myGid),
	}

	if eo.runAsUid != "" && (eo.alwaysRunAsUid || eo.runAsUid != currentUser.Uid) {
		tUid, err := strconv.Atoi(eo.runAsUid)
		if err != nil {
			return errors.Wrapf(err, "converting runas uid %s", eo.runAsUid)
		}
		targetPermissions.Uid = uint32(tUid)
	}

	if eo.runAsGid != "" && (eo.alwaysRunAsGid || eo.runAsGid != currentUser.Gid) {
		tGid, err := strconv.Atoi(eo.runAsGid)
		if err != nil {
			return errors.Wrapf(err, "converting runas gid %s", eo.runAsGid)
		}
		targetPermissions.Gid = uint32(tGid)
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: targetPermissions,
	}

	return nil

}
