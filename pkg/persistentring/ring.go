// Package persistentring provides something akin to a ring buffer, that is persisted to a store. It is intended to be
// used in places we want to to track the last N items, across restarts. Because it writes things to a backing store,
// it is most suitable for mid-sized things, perhaps thousands. It should be bechmarked if size or rate gets too high.
//
// It is implemented as a map, and not a linked list, because it makes the underlying implementation simpler. And since
// we don't insert at arbitrary places, there is little value in a linked list.
//
// Encoding and Decoding are the responsibility of the caller. Creating a generic `any` implementation turns to
// be non-performant because libraries like `gob` need a new decoder per type, which is better handled in the
// caller. See:
//   - https://stackoverflow.com/questions/69874951/re-using-the-same-encoder-decoder-for-the-same-struct-type-in-go-without-creatin
//   - https://github.com/golang/go/issues/29766#issuecomment-454926474
package persistentring

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"

	agenttypes "github.com/kolide/launcher/pkg/agent/types"
)

type storeInt interface {
	agenttypes.GetterSetterDeleterIterator
}

type persistentRing struct {
	store storeInt
	size  uint16
	next  uint16

	lock sync.RWMutex
}

var (
	nextKey = []byte("nextPtr")
)

func New(store storeInt, size uint16) (*persistentRing, error) {
	if size > math.MaxUint16 {
		return nil, fmt.Errorf("size %d too big! Max Uint16", size)
	}

	nextPtr, err := store.Get(nextKey)
	if err != nil {
		return nil, fmt.Errorf("getting next pointer from %s: %w", string(nextKey), err)
	}

	next := byteToInt(nextPtr)

	r := &persistentRing{
		store: store,
		size:  size,
		next:  next,

		lock: sync.RWMutex{},
	}

	return r, nil
}

func (r *persistentRing) Add(val []byte) (err error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.next++
	r.next = r.next % r.size

	nextBytes := intToByte(r.next)

	// TODO: Create MultiSet()
	if err := r.store.Set(nextBytes, val); err != nil {
		return fmt.Errorf("writing value to store (%s): %w", nextBytes, err)
	}
	if err := r.store.Set(nextKey, nextBytes); err != nil {
		return fmt.Errorf("writing next to store (%s: %s): %w", nextKey, nextBytes, err)
	}

	return nil
}

func (r *persistentRing) GetAll() ([][]byte, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	// If we took a callback, we could avoid this allocation.
	results := make([][]byte, 0, r.size)

	stillSeeking := true
	for i := uint16(0); i < r.size; i++ {
		// Start at the pointer _after_ next, so we get oldest first
		pos := (r.next + 1 + i) % r.size

		ptr := intToByte(pos)
		val, err := r.store.Get(ptr)
		if err != nil {
			return nil, fmt.Errorf("getting value from key %s: %w", string(ptr), err)
		}

		// Handling partially filled rings is a hassle.
		// ptr points to the _next_ space, which is the oldest, but it can be nil for too many reasons
		if val == nil {
			if stillSeeking {
				continue
			}

			// If we have some data, then we're not stillSeeking, and we're just done
			break
		}

		// If we have data, we can stop seeking
		stillSeeking = true

		results = append(results, val)
	}

	return results, nil
}

func intToByte(i uint16) []byte {
	bs := make([]byte, binary.MaxVarintLen16)
	binary.LittleEndian.PutUint16(bs, i)
	return bs
}

func byteToInt(b []byte) uint16 {
	if len(b) < 2 {
		// If len(b) is under 2, this panics. So, just return 0 here instead
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}
