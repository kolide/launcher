package remoterestartconsumer

import (
	"errors"
)

type RemoteRestartRequested struct {
	msg string
}

func NewRemoteRestartRequested() RemoteRestartRequested {
	return RemoteRestartRequested{
		msg: "need to reload launcher: remote restart requested",
	}
}

func (e RemoteRestartRequested) Error() string {
	return e.msg
}

func (e RemoteRestartRequested) Is(target error) bool {
	if _, ok := target.(RemoteRestartRequested); ok {
		return true
	}
	return false
}

func IsRemoteRestartRequestedErr(err error) bool {
	return errors.Is(err, RemoteRestartRequested{})
}
