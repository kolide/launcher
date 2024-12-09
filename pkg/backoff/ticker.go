package backoff

import (
	"time"
)

// NewMultiplicativeTicker returns a ticker where each interval = baseDuration * ticks until maxDuration is reached.
func NewMultiplicativeTicker(baseDuration, maxDuration time.Duration) *ticker {
	return newTicker(newMultiplicativeCounter(baseDuration, maxDuration))
}

type ticker struct {
	C               chan time.Time
	baseTicker      *time.Ticker
	stoppedChan     chan struct{}
	stopped         bool
	durationCounter durationCounter
}

func newTicker(durationCounter *durationCounter) *ticker {
	thisTicker := &ticker{
		C:               make(chan time.Time),
		stoppedChan:     make(chan struct{}),
		durationCounter: *durationCounter,
	}

	thisTicker.baseTicker = time.NewTicker(thisTicker.durationCounter.next())

	go func() {
		for {
			select {
			case t := <-thisTicker.baseTicker.C:
				thisTicker.baseTicker.Reset(thisTicker.durationCounter.next())
				thisTicker.C <- t
			case <-thisTicker.stoppedChan:
				thisTicker.baseTicker.Stop()
				return
			}
		}
	}()

	return thisTicker
}

func (t *ticker) Stop() {
	if t.stopped {
		return
	}

	t.stopped = true
	close(t.stoppedChan)
}

type durationCounter struct {
	count                     int
	baseInterval, maxInterval time.Duration
	calcNext                  func(count int, baseDuration time.Duration) time.Duration
}

func (dc *durationCounter) next() time.Duration {
	dc.count++
	interval := dc.calcNext(dc.count, dc.baseInterval)
	if interval > dc.maxInterval {
		return dc.maxInterval
	}
	return interval
}

func newMultiplicativeCounter(baseDuration, maxDuration time.Duration) *durationCounter {
	return &durationCounter{
		baseInterval: baseDuration,
		maxInterval:  maxDuration,
		calcNext: func(count int, baseInterval time.Duration) time.Duration {
			return baseInterval * time.Duration(count)
		},
	}
}
