//go:build !darwin
// +build !darwin

package task

import "time"

// task simply wraps a Go Ticker and uses its facilities for scheduling tasks
type task struct {
	identifier string
	repeats    bool
	interval   time.Duration
	ticker     *time.Ticker
}

func New(identifier string, opts ...Opt) *task {
	t := &task{
		identifier: identifier,
	}

	for _, opt := range opts {
		opt(t)
	}

	t.ticker = time.NewTicker(t.interval)

	return t
}

func (t *task) Stop() {
	t.ticker.Stop()
}

func (t *task) Reset(d time.Duration) {
	t.ticker.Reset(d)
}

func (t *task) C() <-chan time.Time {
	return t.ticker.C
}
