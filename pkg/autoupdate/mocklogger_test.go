package autoupdate

import (
	"sync"
)

type mockLogger struct {
	count int
	mx    sync.Mutex
}

func (l *mockLogger) Log(keyvals ...interface{}) error {
	l.mx.Lock()
	defer l.mx.Unlock()

	l.count = l.count + 1
	return nil
}

func (l *mockLogger) Count() int {
	l.mx.Lock()
	defer l.mx.Unlock()

	return l.count
}
