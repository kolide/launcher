package persistentring

type inMemoryRing struct {
	data [][]byte
	ptr  uint16
}

func NewInMemory(size uint16) *inMemoryRing {
	return &inMemoryRing{
		data: make([][]byte, size, size),
	}
}

func (r *inMemoryRing) Add(val []byte) (err error) {
	r.ptr = r.ptr % uint16(cap(r.data))
	r.data[r.ptr] = make([]byte, len(val))
	copy(r.data[r.ptr], val)
	r.ptr++

	return nil
}

func (r *inMemoryRing) GetAll() ([][]byte, error) {
	results := make([][]byte, 0, cap(r.data))

	stillSeeking := true
	for i := uint16(0); i < uint16(cap(r.data)); i++ {
		// Start at the pointer _after_ next, so we get oldest first
		pos := (r.ptr + i) % uint16(cap(r.data))

		// Handling partially filled rings is a hassle.
		// ptr points to the _next_ space, which is the oldest, but it can be nil for too many reasons
		if r.data[pos] == nil {
			if stillSeeking {
				continue
			}

			// If we have some data, then we're not stillSeeking, and we're just done
			break
		}

		// If we have data, we can stop seeking
		stillSeeking = true

		results = append(results, r.data[pos])
	}

	return results, nil
}
