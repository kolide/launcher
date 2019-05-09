package authenticode

import (
	"context"
	"os/exec"
)

// signtoolOptions are the options for how we call signtool.exe. These
// are *not* the tool options, but instead our own representation of
// the arguments.w
type signtoolOptions struct {
	subjectName    string // If present, use this as the `/n` argument
	skipValidation bool
	signtoolPath   string

	execCC func(context.Context, string, ...string) *exec.Cmd // Allows test overrides

}

type SigntoolOpt func(*signtoolOptions)

func SkipValidation() SigntoolOpt {
	return func(so *signtoolOptions) {
		so.skipValidation = true
	}
}

func WithSubjectName(sn string) SigntoolOpt {
	return func(so *signtoolOptions) {
		so.subjectName = sn
	}
}

func WithSigntoolPath(path string) SigntoolOpt {
	return func(so *signtoolOptions) {
		so.signtoolPath = path
	}
}
