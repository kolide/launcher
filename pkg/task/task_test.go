package task

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
		// 	interval: ,
		// 	// subsystem: "",
		// 	// c:         &mockConsumer{},
		// },
		{
			name:                   "happy path",
			identifier:             "com.kolide.launcher.test",
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

			timesPerformed := countTimesPerformed(task)

			assert.Equal(t, tt.expectedTimesPerformed, timesPerformed)
		})
	}
}

func countTimesPerformed(t Task) int {
	var timesPerformed int
	for {
		select {
		case <-t.C():
			timesPerformed = timesPerformed + 1
			if timesPerformed > 1 {
				return timesPerformed
			}
		}
	}
}
