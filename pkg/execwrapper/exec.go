// +build !windows

package execwrapper

import (
	"context"
	"syscall"
)

func Exec(ctx context.Context, argv0 string, argv []string, envv []string) (err error) {
	return syscall.Exec(argv0, argv, envv)
}
