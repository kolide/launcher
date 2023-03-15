package threadsafebuffer

import (
	"bytes"
	"sync"
)

// thank you zupa https://stackoverflow.com/a/36226525
type ThreadSafeBuffer struct {
	b bytes.Buffer
	m sync.Mutex
}

func (t *ThreadSafeBuffer) Read(p []byte) (n int, err error) {
	t.m.Lock()
	defer t.m.Unlock()
	return t.b.Read(p)
}

func (t *ThreadSafeBuffer) Write(p []byte) (n int, err error) {
	t.m.Lock()
	defer t.m.Unlock()
	return t.b.Write(p)
}

func (t *ThreadSafeBuffer) String() string {
	t.m.Lock()
	defer t.m.Unlock()
	return t.b.String()
}
