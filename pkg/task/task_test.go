package task

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTaskNoRepeat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		identifier             string
		interval               time.Duration
		expectedTimesPerformed int
	}{
		// {
		// 	name: "zero interval",
		// },
		{
			name:                   "happy path",
			identifier:             "TestTaskNoRepeat",
			interval:               time.Second * 2,
			expectedTimesPerformed: 1,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			task := New(
				tt.identifier,
				WithInterval(tt.interval))
			defer task.Stop()

			timesPerformed := countTimesPerformed(task, 1)

			assert.Equal(t, tt.expectedTimesPerformed, timesPerformed)
		})
	}
}

func TestTaskRepeats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		identifier             string
		interval               time.Duration
		expectedTimesPerformed int
	}{
		// {
		// 	name: "zero interval",
		// },
		{
			name:                   "happy path",
			identifier:             "TestTaskRepeats",
			interval:               time.Second * 2,
			expectedTimesPerformed: 2,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			task := New(
				tt.identifier,
				Repeats(),
				WithInterval(tt.interval))
			defer task.Stop()

			timesPerformed := countTimesPerformed(task, 2)

			assert.Equal(t, tt.expectedTimesPerformed, timesPerformed)
		})
	}
}

func TestTaskStop(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		identifier             string
		interval               time.Duration
		expectedTimesPerformed int
	}{
		{
			name:                   "happy path",
			identifier:             "TestTaskStop",
			interval:               time.Second * 2,
			expectedTimesPerformed: 2,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			task := New(
				tt.identifier,
				Repeats(),
				WithInterval(tt.interval))
			task.Stop()

			// timesPerformed := countTimesPerformed(task)

			//  assert.Equal(t, tt.expectedTimesPerformed, timesPerformed)
		})
	}
}

func TestTaskReset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		identifier             string
		interval               time.Duration
		resetInterval          time.Duration
		expectedTimesPerformed int
	}{
		{
			name:                   "happy path",
			identifier:             "TestTaskReset",
			interval:               time.Second * 2,
			expectedTimesPerformed: 2,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			task := New(
				tt.identifier,
				Repeats(),
				WithInterval(tt.interval))
			task.Reset(tt.resetInterval)

			// timesPerformed := countTimesPerformed(task)

			//  assert.Equal(t, tt.expectedTimesPerformed, timesPerformed)
		})
	}
}

func countTimesPerformed(t Task, max int) int {
	var timesPerformed int
	for {
		select {
		case <-t.C():
			timesPerformed = timesPerformed + 1
			if timesPerformed >= max {
				return timesPerformed
			}
		}
	}
}
