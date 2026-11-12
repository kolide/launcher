package backoff

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

var errSentinel = errors.New("sentinel")

func TestWaitFor(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name                          string
		fnDuration, timeout, interval time.Duration
		failures, wantCalls           int
		wantErr                       bool
	}{
		{
			name:      "succeeds first try",
			timeout:   100 * time.Millisecond,
			interval:  10 * time.Millisecond,
			wantCalls: 1,
		},
		{
			name:      "succeeds after retries",
			failures:  3,
			timeout:   100 * time.Millisecond,
			interval:  10 * time.Millisecond,
			wantCalls: 4,
		},
		{
			name:      "fails at timeout",
			failures:  5,
			timeout:   25 * time.Millisecond,
			interval:  10 * time.Millisecond,
			wantCalls: 4, // always ran 2x, then 2x before timeout
			wantErr:   true,
		},
		{
			name:      "interval exceeds timeout",
			failures:  5,
			timeout:   10 * time.Millisecond,
			interval:  50 * time.Millisecond,
			wantCalls: 2, // always ran 2x
			wantErr:   true,
		},
		{
			name:       "slow fn exceeds timeout",
			fnDuration: 50 * time.Millisecond,
			timeout:    10 * time.Millisecond,
			interval:   1 * time.Millisecond,
			wantCalls:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				calls := 0
				fn := func() error {
					calls++
					time.Sleep(tt.fnDuration)
					if calls <= tt.failures {
						return errSentinel
					}
					return nil
				}

				err := WaitFor(fn, tt.timeout, tt.interval)

				if tt.wantErr {
					require.ErrorIs(t, err, errSentinel)
				} else {
					require.NoError(t, err)
				}
				require.Equal(t, tt.wantCalls, calls)
			})
		})
	}
}

func TestWaitForContextParentCancel(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(25*time.Millisecond, cancel)

		start := time.Now()
		err := WaitForContext(ctx, func(context.Context) error {
			return errSentinel
		}, time.Hour, 10*time.Millisecond)

		require.ErrorIs(t, err, errSentinel)
		require.Less(t, time.Since(start), time.Hour)
	})
}
