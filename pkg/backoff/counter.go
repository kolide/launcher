package backoff

import (
	"time"
)

type durationCounter struct {
	count                     int
	baseInterval, maxInterval time.Duration
	calcNext                  func(count int, baseDuration time.Duration) time.Duration
}

func (dc *durationCounter) Next() time.Duration {
	dc.count++
	interval := dc.calcNext(dc.count, dc.baseInterval)
	if interval > dc.maxInterval {
		return dc.maxInterval
	}
	return interval
}

func (dc *durationCounter) Reset() {
	dc.count = 0
}

func NewMultiplicativeDurationCounter(baseDuration, maxDuration time.Duration) *durationCounter {
	return &durationCounter{
		baseInterval: baseDuration,
		maxInterval:  maxDuration,
		calcNext: func(count int, baseInterval time.Duration) time.Duration {
			return baseInterval * time.Duration(count)
		},
	}
}
