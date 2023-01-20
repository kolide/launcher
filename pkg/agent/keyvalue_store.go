package agent

// Retriever is an interface for getting data from a persistent key/value store.
type Retriever interface {
	Get(key []byte) (value []byte, err error)
}

// Storer is an interface for setting data in a persistent key/value store.
type Storer interface {
	Set(key, value []byte) error
}

// RetrieverStorer is an interface that groups the Get and Set methods.
type RetrieverStorer interface {
	Retriever
	Storer
}
