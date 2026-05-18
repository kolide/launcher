// Package atomic provides convenience wrappers around the standard
// library's sync/atomic primitives for the types launcher uses most
// frequently: bool, time.Duration, and string.
package atomic

import (
	"sync/atomic"
	"time"
)

// Bool is an atomic boolean.
type Bool struct {
	v atomic.Bool
}

// Load atomically loads and returns the value.
func (b *Bool) Load() bool { return b.v.Load() }

// Store atomically stores val.
func (b *Bool) Store(val bool) { b.v.Store(val) }

// Swap atomically stores new and returns the previous value.
func (b *Bool) Swap(val bool) bool { return b.v.Swap(val) }

// Duration is an atomic time.Duration.
type Duration struct {
	v atomic.Int64
}

// NewDuration returns a *Duration initialized to d.
func NewDuration(d time.Duration) *Duration {
	dur := &Duration{}
	dur.Store(d)
	return dur
}

// Load atomically loads and returns the value.
func (d *Duration) Load() time.Duration { return time.Duration(d.v.Load()) }

// Store atomically stores val.
func (d *Duration) Store(val time.Duration) { d.v.Store(int64(val)) }

// String is an atomic string.
//
// The zero value is an empty string.
type String struct {
	v atomic.Value
}

// NewString returns a *String initialized to s.
func NewString(s string) *String {
	str := &String{}
	str.Store(s)
	return str
}

// Load atomically loads and returns the value.
func (s *String) Load() string {
	if v := s.v.Load(); v != nil {
		return v.(string)
	}
	return ""
}

// Store atomically stores val.
func (s *String) Store(val string) { s.v.Store(val) }
