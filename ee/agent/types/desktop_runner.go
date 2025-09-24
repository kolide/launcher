package types

// DesktopProcessRecord represents a desktop process that the runner manages
type DesktopProcessRecord interface {
	// SocketPath returns the path to the Unix socket for communicating with this desktop process
	SocketPath() string
	// Pid returns the process ID of the desktop process
	Pid() int
}

// DesktopRunner interface provides access to desktop process management functionality
type DesktopRunner interface {
	// GetDesktopProcessRecords returns information about currently managed desktop processes
	GetDesktopProcessRecords() []DesktopProcessRecord
	// GetDesktopAuthToken returns the authentication token used for communicating with desktop processes
	GetDesktopAuthToken() string
	// SetDesktopRunner is used by the knapsack to store the desktop runner reference
	SetDesktopRunner(runner DesktopRunner)
}
