package backoff

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMultiplicativeCounter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		baseInterval time.Duration
		maxInterval  time.Duration
		expected     []time.Duration
	}{
		{
			name:         "seconds",
			baseInterval: time.Second,
			maxInterval:  5 * time.Second,
			expected: []time.Duration{
				time.Second,     // 1s
				2 * time.Second, // 2s
				3 * time.Second, // 3s
				4 * time.Second, // 4s
				5 * time.Second, // 5s (max interval)
				5 * time.Second, // capped at max interval
			},
		},
		{
			name:         "minutes",
			baseInterval: time.Minute,
			maxInterval:  3 * time.Minute,
			expected: []time.Duration{
				time.Minute,     // 1m
				2 * time.Minute, // 2m
				3 * time.Minute, // 3m (max interval)
				3 * time.Minute, // capped at max interval
				3 * time.Minute, // capped at max interval
			},
		},
		{
			name:         "combo",
			baseInterval: (1 * time.Minute) + (30 * time.Second),
			maxInterval:  5 * time.Minute,
			expected: []time.Duration{
				(1 * time.Minute) + (30 * time.Second),       // 1m30s
				2 * ((1 * time.Minute) + (30 * time.Second)), // 3m
				3 * ((1 * time.Minute) + (30 * time.Second)), // 4m30s
				5 * time.Minute, // 5m
				5 * time.Minute, // 5m
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ec := newMultiplicativeCounter(tt.baseInterval, tt.maxInterval)
			for _, expected := range tt.expected {
				require.Equal(t, expected, ec.next())
			}
		})
	}
}

// TestMultiplicativeTicker tests the NewMultiplicativeTicker and its behavior.
func TestMultiplicativeTicker(t *testing.T) {
	baseTime := 100 * time.Millisecond
	maxTime := 500 * time.Millisecond

	tk := NewMultiplicativeTicker(baseTime, maxTime)
	defer tk.Stop()

	expectedDurations := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		300 * time.Millisecond,
		400 * time.Millisecond,
		500 * time.Millisecond, // maxTime limit
		500 * time.Millisecond, // maxTime limit
	}

	buffer := 25 * time.Millisecond

	for _, expected := range expectedDurations {
		start := time.Now()

		select {
		case <-tk.C:
			require.WithinDuration(t, start, time.Now(), expected+buffer)
		case <-time.After(maxTime + buffer):
			t.Fatalf("ticker did not send event in expected time: %v", expected)
		}
	}

	// stop the ticker
	tk.Stop()

	// call stop again to make sure no panic (same as stdlib ticker)
	tk.Stop()
}
