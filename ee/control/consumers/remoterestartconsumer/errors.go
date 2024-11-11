package remoterestartconsumer

import (
	"errors"
)

type ErrRemoteRestartRequested struct {
	msg string
}

func NewRemoteRestartRequestedErr() ErrRemoteRestartRequested {
	return ErrRemoteRestartRequested{
		msg: "need to reload launcher: remote restart requested",
	}
}

func (e ErrRemoteRestartRequested) Error() string {
	return e.msg
}

func (e ErrRemoteRestartRequested) Is(target error) bool {
	if _, ok := target.(ErrRemoteRestartRequested); ok {
		return true
	}
	return false
}

func IsRemoteRestartRequestedErr(err error) bool {
	return errors.Is(err, ErrRemoteRestartRequested{})
}
