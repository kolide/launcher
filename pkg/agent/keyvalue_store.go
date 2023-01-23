package agent

// Getter is an interface for getting data from a persistent key/value store.
type Getter interface {
	Get(key []byte) (value []byte, err error)
}

// Setter is an interface for setting data in a persistent key/value store.
type Setter interface {
	Set(key, value []byte) error
}

// GetterSetter is an interface that groups the Get and Set methods.
type GetterSetter interface {
	Getter
	Setter
}
