package remoterestartconsumer

import (
	"errors"
	"fmt"
)

// ErrRemoteRestartRequested is returned to the main launcher rungroup when
// a remote restart has been requested.
type ErrRemoteRestartRequested struct {
	msg   string
	runId string
}

func NewRemoteRestartRequestedErr(runId string) ErrRemoteRestartRequested {
	return ErrRemoteRestartRequested{
		msg:   "need to reload launcher: remote restart requested",
		runId: runId,
	}
}

func (e ErrRemoteRestartRequested) Error() string {
	return fmt.Sprintf("%s (run_id %s)", e.msg, e.runId)
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
