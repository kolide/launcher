package types

// Getter is an interface for getting data from a key/value store.
type Getter interface {
	// Get retrieves the value for a key.
	// Returns a nil value if the key does not exist.
	Get(key []byte) (value []byte, err error)
}

// Setter is an interface for setting data in a key/value store.
type Setter interface {
	// Set sets the value for a key.
	// If the key exist then its previous value will be overwritten.
	// Returns an error if the key is blank, if the key is too large, or if the value is too large.
	Set(key, value []byte) error
}

// Deleter is an interface for deleting data in a key/value store.
type Deleter interface {
	// Delete removes a key.
	// If the key does not exist then nothing is done and a nil error is returned.
	Delete(keys ...[]byte) error
}

// Iterator is an interface for iterating data in a key/value store.
type Iterator interface {
	// ForEach executes a function for each key/value pair in a store.
	// If the provided function returns an error then the iteration is stopped and
	// the error is returned to the caller. The provided function must not modify
	// the store; this will result in undefined behavior.
	ForEach(fn func(k, v []byte) error) error
}

// Updater is an interface for bulk replacing data in a key/value store.
type Updater interface {
	// Update takes a sequence of alternating key-value pairs, and inserts
	// these key-values into the store. Any preexisting keys in the store which
	// do not exist in data will be deleted.
	Update(pairs ...string) ([]string, error)
}

// GetterSetter is an interface that groups the Get and Set methods.
type GetterSetter interface {
	Getter
	Setter
}

// GetterSetterDeleter is an interface that groups the Get, Set, and Delete methods.
type GetterSetterDeleter interface {
	Getter
	Setter
	Deleter
}

// GetterSetterDeleterIterator is an interface that groups the Get, Set, Delete, and ForEach methods.
type GetterSetterDeleterIterator interface {
	Getter
	Setter
	Deleter
	Iterator
}

// GetterSetterDeleterIteratorUpdater is an interface that groups the Get, Set, Delete, ForEach, and Update methods.
type GetterSetterDeleterIteratorUpdater interface {
	Getter
	Setter
	Deleter
	Iterator
	Updater
}

// Convenient alias for a key value store that supports all methods
type KVStore = GetterSetterDeleterIteratorUpdater
