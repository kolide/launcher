//go:build darwin
// +build darwin

package task

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#cgo darwin LDFLAGS: -framework Cocoa
#include "scheduler.h"
*/
import (
	"C"
)
import "time"

type task struct {
	identifier string
	repeats    bool
	interval   time.Duration
	channel    <-chan time.Time
}

func New(identifier string, opts ...Opt) *task {
	c := make(chan time.Time, 1)
	t := &task{
		identifier: identifier,
		channel:    c,
	}

	for _, opt := range opts {
		opt(t)
	}

	return t
}

func (t *task) Stop() {

}

func (t *task) Reset(interval time.Duration) {

}

func (t *task) C() <-chan time.Time {
	return t.channel
}

//export perform
func perform() {
	// getRecommendedUpdates will use this callback to indicate how many updates have been found
	// updatesData = make([]map[string]interface{}, numUpdates)
}
