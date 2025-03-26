package errgroup

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kolide/launcher/ee/gowrapper"
	"github.com/kolide/launcher/pkg/traces"
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
	maxErrgroupShutdownDuration  = 30 * time.Second
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
	l.errgroup.Go(func() (err error) {
		slogger := l.slogger.With("goroutine_name", goroutineName)

		// Catch any panicking goroutines and log them. We also want to make sure
		// we return an error from this goroutine overall if it panics.
		defer func() {
			if r := recover(); r != nil {
				slogger.Log(ctx, slog.LevelError,
					"panic occurred in goroutine",
					"err", r,
				)
				if recoveredErr, ok := r.(error); ok {
					slogger.Log(ctx, slog.LevelError,
						"panic stack trace",
						"stack_trace", fmt.Sprintf("%+v", errors.WithStack(recoveredErr)),
					)
					err = recoveredErr
				}
			}
		}()

		slogger.Log(ctx, slog.LevelInfo,
			"starting goroutine in errgroup",
		)

		err = goroutine()

		slogger.Log(ctx, slog.LevelInfo,
			"exiting goroutine in errgroup",
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
		slogger := l.slogger.With("goroutine_name", goroutineName)

		if delay != 0*time.Second {
			select {
			case <-time.After(delay):
				slogger.Log(ctx, slog.LevelDebug,
					"exiting delay before starting repeated goroutine",
				)
			case <-l.doneCtx.Done():
				return nil
			}
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			// Run goroutine immediately
			if err := goroutine(); err != nil {
				slogger.Log(ctx, slog.LevelInfo,
					"exiting repeated goroutine in errgroup due to error",
					"goroutine_err", err,
				)
				return err
			}

			// Wait for next interval or for errgroup shutdown
			select {
			case <-l.doneCtx.Done():
				slogger.Log(ctx, slog.LevelInfo,
					"exiting repeated goroutine in errgroup due to shutdown",
				)
				return nil
			case <-ticker.C:
				continue
			}
		}
	})
}

// AddShutdownGoroutine adds the given goroutine to the errgroup, ensuring that we log its start and exit.
// The goroutine will not execute until the errgroup has received a signal to exit.
func (l *LoggedErrgroup) AddShutdownGoroutine(ctx context.Context, goroutineName string, goroutine func() error) {
	l.errgroup.Go(func() error {
		slogger := l.slogger.With("goroutine_name", goroutineName)

		// Catch any panicking goroutines and log them. We do not want to return
		// the error from this routine, as we do for StartGoroutine and StartRepeatedGoroutine --
		// shutdown goroutines should not return an error besides the errgroup's initial error.
		defer func() {
			if r := recover(); r != nil {
				slogger.Log(ctx, slog.LevelError,
					"panic occurred in shutdown goroutine",
					"err", r,
				)
				if err, ok := r.(error); ok {
					slogger.Log(ctx, slog.LevelError,
						"panic stack trace",
						"stack_trace", fmt.Sprintf("%+v", errors.WithStack(err)),
					)
				}
			}
		}()

		// Wait for errgroup to exit
		<-l.doneCtx.Done()

		ctx, span := traces.StartSpan(ctx, "goroutine_name", goroutineName)
		defer span.End()

		slogger.Log(ctx, slog.LevelInfo,
			"starting shutdown goroutine in errgroup",
		)

		goroutineStart := time.Now()
		err := goroutine()
		elapsedTime := time.Since(goroutineStart)

		logLevel := slog.LevelInfo
		if elapsedTime > maxShutdownGoroutineDuration || err != nil {
			logLevel = slog.LevelWarn
		}
		slogger.Log(ctx, logLevel,
			"exiting shutdown goroutine in errgroup",
			"goroutine_run_time", elapsedTime.String(),
			"goroutine_err", err,
		)

		// We don't want to actually return the error here, to avoid causing an otherwise successful call
		// to `Shutdown` => `Wait` to return an error. Shutdown routine errors don't matter for the success
		// of the errgroup overall.
		return l.doneCtx.Err()
	})
}

func (l *LoggedErrgroup) Shutdown() {
	l.cancel()
}

func (l *LoggedErrgroup) Wait(ctx context.Context) error {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	errChan := make(chan error)
	gowrapper.Go(ctx, l.slogger, func() {
		errChan <- l.errgroup.Wait()
	})

	// Wait to receive an error from l.errgroup.Wait(), but only until our shutdown timeout.
	select {
	case err := <-errChan:
		return err
	case <-time.After(maxErrgroupShutdownDuration):
		l.slogger.Log(ctx, slog.LevelWarn,
			"errgroup did not complete shutdown within timeout",
			"timeout", maxErrgroupShutdownDuration.String(),
		)
		return nil
	}
}

func (l *LoggedErrgroup) Exited() <-chan struct{} {
	return l.doneCtx.Done()
}
