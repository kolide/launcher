// Package execwrapper provides a Exec method that should work on
// posix or windows systems.
//
// Windows does not support syscall.Exec. This acts as a wrapper
// around os/exec to provide similar functionality.
package execwrapper
