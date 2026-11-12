package backoff

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/time/rate"
)

// WaitFor is a wrapper over the stock go limiter pattern. It can be used to
// retry calling fn until it returns nil or timeout would be reached before another
// attempt. It is not an exponential backoff, and is intended for use in simple
// startup logic.
//
// fn is called immediately once. If it fails, a second attempt occurs without delay.
// Subsequent attempts are spaced by interval. nil is always returned if fn returns
// nil.
func WaitFor(fn func() error, timeout, interval time.Duration) error {
	return WaitForContext(
		context.Background(),
		func(_ context.Context) error {
			return fn()
		},
		timeout,
		interval,
	)
}

// WaitForContext is a wrapper over the stock go limiter pattern. It can be used to
// retry calling fn until it returns nil, the context is cancelled, or timeout would
// be reached before another attempt. It is not an exponential backoff, and is intended
// for use in simple startup logic.
//
// fn is called immediately once. If it fails, a second attempt occurs without delay.
// Subsequent attempts are spaced by interval. nil is always returned if fn returns
// nil.
//
// Provided context's cancellation is honored over retry logic.
func WaitForContext(ctx context.Context, fn func(ctx context.Context) error, timeout, interval time.Duration) error {
	deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()

	limiter := rate.NewLimiter(rate.Every(interval), 1)
	counter := 0
	for {
		counter += 1

		err := fn(deadlineCtx)

		if err == nil {
			return nil
		}

		limitErr := limiter.Wait(deadlineCtx)

		switch {
		case ctx.Err() != nil:
			return fmt.Errorf("context done after %s (%d attempts) due to %q, last fn error: %w", time.Since(start), counter, ctx.Err(), err)
		case limitErr != nil:
			return fmt.Errorf("timeout after %s (%d attempts) due to %q, last fn error: %w", time.Since(start), counter, limitErr, err)
		}
	}
}
