// Package persistentring provides something akin to a ring buffer, that is persisted to a store. It is intended to be
// used in places we want to to track the last N items, across restarts. It should be suitable for low volume things,
// where N is measured in hundreds.
//
// The underlying implementation is not as efficient as a pure ring buffer, it is much more analogous to a map. There
// is no deletion, changing key sizes may cause old data to appear.
package persistentring

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"sync"

	agenttypes "github.com/kolide/launcher/pkg/agent/types"
)

type storeInt interface {
	agenttypes.GetterSetterDeleterIterator
}

type persistentRing struct {
	store storeInt
	size  int
	next  int

	lock sync.RWMutex
}

var (
	nextKey = []byte("nextPtr")
)

func New(store storeInt, size int) (*persistentRing, error) {
	nextPtr, err := store.Get(nextKey)
	if err != nil {
		return nil, fmt.Errorf("getting next pointer: %w", err)
	}

	next, err := byteToInt(nextPtr)
	if err != nil {
		return nil, fmt.Errorf("converting next (%s) to int: %w", nextPtr, err)
	}

	r := &persistentRing{
		store: store,
		size:  size,
		next:  next,
		lock:  sync.RWMutex{},
	}

	return r, nil
}

// Write writes a value to the ring.
func (r *persistentRing) Write(val []byte) (n int, err error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.next++

	nextBytes, err := intToByte(r.next % r.size)
	if err != nil {
		return 0, fmt.Errorf("converting %d to bytes: %w", r.next, err)
	}

	// TODO: Create MultiSet()
	if err := r.store.Set(nextBytes, val); err != nil {
		return 0, fmt.Errorf("writing value to store: %w", err)
	}
	if err := r.store.Set(nextKey, nextBytes); err != nil {
		return 0, fmt.Errorf("writing next to store: %w", err)
	}

	return len(val), nil
}

func (r *persistentRing) GetAll() ([][]byte, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	results := make([][]byte, r.size)

	for i := 0; i < r.size; i++ {
		// TODO: Modulus
		// Start at the pointer _after_ next, so we get oldest first
		ptr, err := intToByte(r.next + 1 + i)
		if err != nil {
			return nil, fmt.Errorf("converting %d to bytes: %w", r.next+1+i, err)
		}
		val, err := r.store.Get(ptr)
		if err != nil {
			return nil, fmt.Errorf("getting value: %w", err)
		}
		results[i] = val
	}

	return results, nil
}

// Close does nothing, but allows us to implement io.Closer
func (r *persistentRing) Close() error {
	return nil

}

func intToByte(i int) ([]byte, error) {
	// Allocating a bytes.Buffer is a bit of a bummer here, but with the
	// eventual destination needing []byte as a value for a key, the overhead
	// feels unavoidable.
	var b bytes.Buffer
	err := gob.NewEncoder(&b).Encode(i)
	return b.Bytes(), err
}

func byteToInt(b []byte) (int, error) {
	var i int
	return i, gob.NewDecoder(bytes.NewReader(b)).Decode(&i)
}
