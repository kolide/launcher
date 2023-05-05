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
	r.ptr++
	r.ptr = r.ptr % uint16(cap(r.data))

	r.data[r.ptr] = val

	return nil
}

func (r *inMemoryRing) GetAll() ([][]byte, error) {
	results := make([][]byte, cap(r.data))

	for i := uint16(0); i < uint16(cap(r.data)); i++ {
		// Start at the pointer _after_ next, so we get oldest first
		pos := (r.ptr + 1 + i) % uint16(cap(r.data))

		results[i] = r.data[pos]
	}

	return results, nil
}
