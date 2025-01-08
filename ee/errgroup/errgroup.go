package errgroup

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

type LoggedErrgroup struct {
	errgroup *errgroup.Group
	cancel   context.CancelFunc
	doneCtx  context.Context // nolint:containedctx
	slogger  *slog.Logger
}

const (
	maxShutdownGoroutineDuration = 3 * time.Second
)

func NewLoggedErrgroup(ctx context.Context, slogger *slog.Logger) *LoggedErrgroup {
	ctx, cancel := context.WithCancel(ctx)
	e, doneCtx := errgroup.WithContext(ctx)

	return &LoggedErrgroup{
		errgroup: e,
		cancel:   cancel,
		doneCtx:  doneCtx,
		slogger:  slogger,
	}
}

// StartGoroutine starts the given goroutine in the errgroup, ensuring that we log its start and exit.
func (l *LoggedErrgroup) StartGoroutine(ctx context.Context, goroutineName string, goroutine func() error) {
	l.errgroup.Go(func() error {
		// Catch any panicking goroutines and log them
		defer func() {
			if r := recover(); r != nil {
				l.slogger.Log(ctx, slog.LevelError,
					"panic occurred in goroutine",
					"err", r,
				)
				if err, ok := r.(error); ok {
					l.slogger.Log(ctx, slog.LevelError,
						"panic stack trace",
						"stack_trace", fmt.Sprintf("%+v", errors.WithStack(err)),
					)
				}
			}
		}()

		l.slogger.Log(ctx, slog.LevelInfo,
			"starting goroutine in errgroup",
			"goroutine_name", goroutineName,
		)

		err := goroutine()

		l.slogger.Log(ctx, slog.LevelInfo,
			"exiting goroutine in errgroup",
			"goroutine_name", goroutineName,
			"goroutine_err", err,
		)

		return err
	})
}

// StartRepeatedGoroutine starts the given goroutine in the errgroup, ensuring that we log its start and exit.
// If the delay is non-zero, the goroutine will not start until after the delay interval has elapsed. The goroutine
// will run on the given interval, and will continue to run until it returns an error or the errgroup shuts down.
func (l *LoggedErrgroup) StartRepeatedGoroutine(ctx context.Context, goroutineName string, interval time.Duration, delay time.Duration, goroutine func() error) {
	l.StartGoroutine(ctx, goroutineName, func() error {
		if delay != 0*time.Second {
			select {
			case <-time.After(delay):
				l.slogger.Log(ctx, slog.LevelDebug,
					"exiting delay before starting repeated goroutine",
					"goroutine_name", goroutineName,
				)
			case <-l.doneCtx.Done():
				return nil
			}
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-l.doneCtx.Done():
				l.slogger.Log(ctx, slog.LevelInfo,
					"exiting repeated goroutine in errgroup due to shutdown",
					"goroutine_name", goroutineName,
				)
				return nil
			case <-ticker.C:
				if err := goroutine(); err != nil {
					l.slogger.Log(ctx, slog.LevelInfo,
						"exiting repeated goroutine in errgroup due to error",
						"goroutine_name", goroutineName,
						"goroutine_err", err,
					)
					return err
				}
			}
		}
	})
}

// AddShutdownGoroutine adds the given goroutine to the errgroup, ensuring that we log its start and exit.
// The goroutine will not execute until the errgroup has received a signal to exit.
func (l *LoggedErrgroup) AddShutdownGoroutine(ctx context.Context, goroutineName string, goroutine func() error) {
	l.StartGoroutine(ctx, goroutineName, func() error {
		// Wait for errgroup to exit
		<-l.doneCtx.Done()

		goroutineStart := time.Now()
		err := goroutine()
		elapsedTime := time.Since(goroutineStart)

		// Log anything amiss about the shutdown goroutine -- did it return an error? Did it take too long?
		if err != nil {
			l.slogger.Log(ctx, slog.LevelWarn,
				"shutdown routine returned err",
				"goroutine_name", goroutineName,
				"goroutine_run_time", elapsedTime.String(),
				"goroutine_err", err,
			)
		} else if elapsedTime > maxShutdownGoroutineDuration {
			l.slogger.Log(ctx, slog.LevelWarn,
				"noticed slow shutdown routine",
				"goroutine_name", goroutineName,
				"goroutine_run_time", elapsedTime.String(),
			)
		}

		// We don't want to actually return the error here, to avoid causing an otherwise successful call
		// to `Shutdown` => `Wait` to return an error. Shutdown routine errors don't matter for the success
		// of the errgroup overall.
		return l.doneCtx.Err()
	})
}

func (l *LoggedErrgroup) Shutdown() {
	l.cancel()
}

func (l *LoggedErrgroup) Wait() error {
	return l.errgroup.Wait()
}

func (l *LoggedErrgroup) Exited() <-chan struct{} {
	return l.doneCtx.Done()
}
