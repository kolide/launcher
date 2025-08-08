package multislogger

import "io"

// closerFunc adapts a function to the io.Closer interface.
type closerFunc func() error

func (f closerFunc) Close() error { return f() }

// combineClosers returns a Closer that calls Close on each provided closer in order.
// It returns the first non-nil error (if any) encountered.
func combineClosers(closers ...io.Closer) io.Closer {
	return closerFunc(func() error {
		var firstErr error
		for _, c := range closers {
			if c == nil {
				continue
			}
			if err := c.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	})
}
