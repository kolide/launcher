//go:build !windows

package execwrapper

import (
	"context"
	"log/slog"
	"syscall"
)

func Exec(_ context.Context, _ *slog.Logger, argv0 string, argv []string, envv []string, _ bool) (err error) {
	return syscall.Exec(argv0, argv, envv)
}
