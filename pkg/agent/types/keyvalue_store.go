package types

// Getter is an interface for getting data from a persistent key/value store.
type Getter interface {
	Get(key []byte) (value []byte, err error)
}

// Setter is an interface for setting data in a persistent key/value store.
type Setter interface {
	Set(key, value []byte) error
}

// Deleter is an interface for deleting data in a persistent key/value store.
type Deleter interface {
	Delete(key []byte) error
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
