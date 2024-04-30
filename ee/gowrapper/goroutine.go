package gowrapper

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pkg/errors"
)

// Go is a thin wrapper around `go func()` that ensures we log panics
// and then handle them appropriately. `onPanic` defines the behavior
// that should happen after recovering from a panic.
func Go(ctx context.Context, slogger *slog.Logger, goroutine func(), onPanic func(r any)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slogger.Log(ctx, slog.LevelError,
					"panic occurred in goroutine",
					"err", r,
				)
				if err, ok := r.(error); ok {
					slogger.Log(ctx, slog.LevelError,
						"panic stack trace",
						"stack_trace", fmt.Sprintf("%+v", errors.WithStack(err)),
					)
				}

				onPanic(r)
			}
		}()

		goroutine()
	}()
}
