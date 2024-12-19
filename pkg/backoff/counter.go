package backoff

import (
	"time"
)

type durationCounter struct {
	count                     int
	baseInterval, maxInterval time.Duration
	calcNext                  func(count int, baseDuration time.Duration) time.Duration
}

// Next increments the count and returns the base interval multiplied by the count.
// If the result is greater than the maxDuration, maxDuration is returned.
func (dc *durationCounter) Next() time.Duration {
	dc.count++
	interval := dc.calcNext(dc.count, dc.baseInterval)
	if interval > dc.maxInterval {
		return dc.maxInterval
	}
	return interval
}

// Reset resets the count to 0.
func (dc *durationCounter) Reset() {
	dc.count = 0
}

// NewMultiplicativeDurationCounter creates a new durationCounter that multiplies the base interval by the count.
// Count is incremented each time Next() is called and returns the base interval multiplied by the count.
// If the result is greater than the maxDuration, maxDuration is returned.
func NewMultiplicativeDurationCounter(baseDuration, maxDuration time.Duration) *durationCounter {
	return &durationCounter{
		baseInterval: baseDuration,
		maxInterval:  maxDuration,
		calcNext: func(count int, baseInterval time.Duration) time.Duration {
			return baseInterval * time.Duration(count)
		},
	}
}
