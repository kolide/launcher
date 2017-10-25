package log

import (
	"os"
	"sync"
	"testing"
)

func TestConcurrentLogging(t *testing.T) {
	l := NewLogger(os.Stderr)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			for j := 0; j < 10; j++ {
				l.Info(i, j)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
}
