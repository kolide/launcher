package task

import "time"

// Task is an interface for scheduling tasks to be performed.
type Task interface {
	// Prevents the task from being scheduled again.
	Stop()
	// Reset stops a task and resets its interval to the specified duration.
	Reset(interval time.Duration)
	// The channel on which the task schedule is delivered.
	C() <-chan time.Time
}

type Opt func(*task)

// Repeats reschedules the task at the specified interval after finishing.
func Repeats() Opt {
	return func(t *task) {
		t.repeats = true
	}
}

// WithInterval is the average interval between invocations of the task.
func WithInterval(interval time.Duration) Opt {
	return func(t *task) {
		t.interval = interval
	}
}
