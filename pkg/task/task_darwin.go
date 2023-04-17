//go:build darwin
// +build darwin

package task

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#cgo darwin LDFLAGS: -framework Cocoa

#include <stdint.h>

void schedule(char*, int, uint64_t, void*);
void reset(void*);
void stop(void*);

*/
import "C"
import (
	"sync"
	"time"
	"unsafe"
)

type task struct {
	identifier string
	repeats    bool
	interval   time.Duration
	channel    chan time.Time
	activity   unsafe.Pointer
}

var (
	tasks map[string]*task = make(map[string]*task)
	mu    sync.RWMutex
)

func New(identifier string, opts ...Opt) *task {
	mu.Lock()
	defer mu.Unlock()

	existingTask, ok := tasks[identifier]
	if ok {
		return existingTask
	}

	c := make(chan time.Time, 1)
	t := &task{
		identifier: identifier,
		channel:    c,
	}

	for _, opt := range opts {
		opt(t)
	}

	tasks[identifier] = t

	identifierCStr := C.CString(identifier)
	// defer C.free(unsafe.Pointer(identifierCStr))
	defer C.schedule(identifierCStr, C.int(intVal(t.repeats)), C.ulonglong(t.interval), t.activity)

	return t
}

func (t *task) Stop() {
	C.stop(t.activity)
}

func (t *task) Reset(interval time.Duration) {
	C.reset(t.activity)
}

func (t *task) C() <-chan time.Time {
	return t.channel
}

//export performTask
func performTask(identifier *C.char) {
	if identifier != nil {
		mu.RLock()
		defer mu.RUnlock()

		existingTask, ok := tasks[C.GoString(identifier)]
		if ok {
			sendTime(existingTask.channel)
		}
	}
}

// sendTime does a non-blocking send of the current time on c.
func sendTime(c chan time.Time) {
	select {
	case c <- time.Now():
	default:
	}
}

func intVal(value bool) int {
	var iVal int
	if value {
		iVal = 1
	}
	return iVal
}
