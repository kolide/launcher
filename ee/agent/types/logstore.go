package types

// RowDeleter is an interface for deleting rows by rowid in a sql store
type RowDeleter interface {
	DeleteRows(rowids ...any) error
}

// TimestampedIterator is a read-only interface for iterating timestamped data.
type TimestampedIterator interface {
	// ForEach executes a function for each timestamp/value pair in a store.
	// If the provided function returns an error then the iteration is stopped and
	// the error is returned to the caller. The provided function must not modify
	// the store; this will result in undefined behavior.
	ForEach(fn func(rowid, timestamp int64, v []byte) error) error
}

// TimestampedAppender is an interface for supporting the addition of timestamped values to a store
type TimestampedAppender interface {
	// AppendValue takes the timestamp, and marshalled value for insertion as a new row
	AppendValue(timestamp int64, value []byte) error
}

// TimestampedIteratorDeleterAppenderCloser is an interface to support the storage and retrieval of
// sets of timestamped values. This can be used where a strict key/value interface may not suffice,
// e.g. for writing logs or historical records to sqlite
type TimestampedIteratorDeleterAppenderCloser interface {
	TimestampedIterator
	TimestampedAppender
	RowDeleter
	Closer
}

// LogStore is a convenient alias for a store that supports all methods required to manipulate sqlite logs
type LogStore = TimestampedIteratorDeleterAppenderCloser
