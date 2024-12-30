package errgroup

import (
	"context"
	"log/slog"
	"time"

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

// AddGoroutineToErrgroup adds the given goroutine to the errgroup, ensuring that we log its start and exit.
func (l *LoggedErrgroup) AddGoroutineToErrgroup(ctx context.Context, goroutineName string, goroutine func() error) {
	l.errgroup.Go(func() error {
		l.slogger.Log(ctx, slog.LevelInfo,
			"starting goroutine in errgroup",
			"goroutine_name", goroutineName,
		)

		goroutineStart := time.Now()
		err := goroutine()
		elapsedTime := time.Since(goroutineStart)

		l.slogger.Log(ctx, slog.LevelInfo,
			"exiting goroutine in errgroup",
			"goroutine_name", goroutineName,
			"goroutine_run_time", elapsedTime.String(),
			"goroutine_err", err,
		)

		return err
	})
}

// AddRepeatedGoroutineToErrgroup adds the given goroutine to the errgroup, ensuring that we log its start and exit.
// If the delay is non-zero, the goroutine will not start until after the delay interval has elapsed. The goroutine
// will run on the given interval, and will continue to run until it returns an error or the errgroup shuts down.
func (l *LoggedErrgroup) AddRepeatedGoroutineToErrgroup(ctx context.Context, goroutineName string, interval time.Duration, delay time.Duration, goroutine func() error) {
	l.errgroup.Go(func() error {
		l.slogger.Log(ctx, slog.LevelInfo,
			"starting repeated goroutine in errgroup",
			"goroutine_name", goroutineName,
			"goroutine_interval", interval.String(),
			"goroutine_start_delay", delay.String(),
		)

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
					"exiting repeated goroutine in errgroup",
					"goroutine_name", goroutineName,
				)
				return nil
			case <-ticker.C:
				goroutineStart := time.Now()
				err := goroutine()
				elapsedTime := time.Since(goroutineStart)

				if err != nil {
					l.slogger.Log(ctx, slog.LevelInfo,
						"exiting repeated goroutine in errgroup",
						"goroutine_name", goroutineName,
						"goroutine_run_time", elapsedTime.String(),
						"goroutine_err", err,
					)
					return err
				}
			}
		}
	})
}

// AddShutdownGoroutineToErrgroup adds the given goroutine to the errgroup, ensuring that we log its start and exit.
// The goroutine will not execute until the errgroup has received a signal to exit.
func (l *LoggedErrgroup) AddShutdownGoroutineToErrgroup(ctx context.Context, goroutineName string, goroutine func() error) {
	l.errgroup.Go(func() error {
		// Wait for errgroup to exit
		<-l.doneCtx.Done()

		l.slogger.Log(ctx, slog.LevelInfo,
			"starting shutdown goroutine in errgroup",
			"goroutine_name", goroutineName,
		)

		goroutineStart := time.Now()
		err := goroutine()
		elapsedTime := time.Since(goroutineStart)

		logLevel := slog.LevelInfo
		if elapsedTime > maxShutdownGoroutineDuration {
			logLevel = slog.LevelWarn
		}

		l.slogger.Log(ctx, logLevel,
			"exiting shutdown goroutine in errgroup",
			"goroutine_name", goroutineName,
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

func (l *LoggedErrgroup) Wait() error {
	return l.errgroup.Wait()
}

func (l *LoggedErrgroup) Exited() <-chan struct{} {
	return l.doneCtx.Done()
}
