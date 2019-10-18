package autoupdate

import "github.com/pkg/errors"

type LauncherRestartNeeded struct {
	msg string
}

func NewLauncherRestartNeededErr(msg string) LauncherRestartNeeded {
	return LauncherRestartNeeded{
		msg: msg,
	}
}

func (e LauncherRestartNeeded) Error() string {
	return e.msg
}

func IsLauncherRestartNeededErr(err error) bool {
	_, ok := errors.Cause(err).(LauncherRestartNeeded)
	return ok
}
