package launcher

import (
	"errors"
	"fmt"
)

// InfoCmd is a launcher parsing error indicating that launcher was called with an informational command
// as a flag (for example, --version or --dev_help), and that launcher execution should not proceed.
// This allows us to print the information and exit gracefully.
type InfoCmd struct {
	msg string
}

func NewInfoCmdError(cmd string) InfoCmd {
	return InfoCmd{
		msg: fmt.Sprintf("launcher called with info cmd %s", cmd),
	}
}

func (e InfoCmd) Error() string {
	return e.msg
}

func (e InfoCmd) Is(target error) bool {
	if _, ok := target.(InfoCmd); ok {
		return true
	}
	return false
}

func IsInfoCmd(err error) bool {
	return errors.Is(err, InfoCmd{})
}
