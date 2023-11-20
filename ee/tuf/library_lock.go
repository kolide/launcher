package tuf

import "sync"

// libraryLock wraps a number of locks, ensuring that any one binary's library
// can only be modified by one routine at a time.
type libraryLock struct {
	locks map[autoupdatableBinary]*sync.Mutex
}

func newLibraryLock() *libraryLock {
	l := make(map[autoupdatableBinary]*sync.Mutex)
	for _, binary := range binaries {
		l[binary] = &sync.Mutex{}
	}

	return &libraryLock{
		locks: l,
	}
}

func (l *libraryLock) Lock(binary autoupdatableBinary) {
	if binaryLibraryLock, ok := l.locks[binary]; ok {
		binaryLibraryLock.Lock()
	}
}

func (l *libraryLock) Unlock(binary autoupdatableBinary) {
	if binaryLibraryLock, ok := l.locks[binary]; ok {
		binaryLibraryLock.Unlock()
	}
}
