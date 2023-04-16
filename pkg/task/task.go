package task

import "time"

type Task interface {
	Stop()
	Reset(interval time.Duration)
	C() <-chan time.Time
}

type Opt func(*task)

func Repeats() Opt {
	return func(t *task) {
		t.repeats = true
	}
}

func WithInterval(interval time.Duration) Opt {
	return func(t *task) {
		t.interval = interval
	}
}
