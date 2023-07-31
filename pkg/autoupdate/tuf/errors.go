package tuf

import (
	"fmt"

	"github.com/pkg/errors"
)

type LauncherReloadNeeded struct {
	msg string
}

func NewLauncherReloadNeededErr(launcherVersion string) LauncherReloadNeeded {
	return LauncherReloadNeeded{
		msg: fmt.Sprintf("need to reload launcher: new version %s downloaded", launcherVersion),
	}
}

func (e LauncherReloadNeeded) Error() string {
	return e.msg
}

func IsLauncherReloadNeededErr(err error) bool {
	_, ok := errors.Cause(err).(LauncherReloadNeeded)
	return ok
}
