//go:build !windows
// +build !windows

package execwrapper

import (
	"context"
	"log/slog"
	"syscall"
)

func Exec(ctx context.Context, _ *slog.Logger, argv0 string, argv []string, envv []string, isSubCommaand bool) (err error) {
	return syscall.Exec(argv0, argv, envv)
}
