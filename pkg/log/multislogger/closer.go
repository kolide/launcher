package multislogger

// closerFunc adapts a function to the io.Closer interface.
type closerFunc func() error

func (f closerFunc) Close() error { return f() }
