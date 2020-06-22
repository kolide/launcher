// +build !posix

package cmdwrapper

import (
	"errors"
	"os/exec"
)

func (eo *execOptions) setRunAsUser(cmd *exec.Cmd) error {
	return errors.New("not implemented on this platform")
}
