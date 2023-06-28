package diag

import (
	"io"
)

// Checkup encapsulates a launcher health checkup
type Checkup struct {
	name  string
	check func() (string, error)
}

type CheckupRunner interface {
	Name() string
	Run(short io.Writer) (string, error)
}
