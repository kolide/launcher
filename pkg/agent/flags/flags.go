package flags

import "time"

// Flags is an interface for setting and retrieving launcher agent flags.
type Flags interface {
	// Registers an observer to receive messages when the specified keys change.
	RegisterChangeObserver(observer FlagsChangeObserver, keys ...FlagKey)

	// DesktopEnabled causes the launcher desktop process and GUI to be enabled.
	SetDesktopEnabled(enabled bool) error
	DesktopEnabled() bool

	// DebugServerData causes logging and diagnostics related to control server error handling  to be enabled.
	SetDebugServerData(debug bool) error
	DebugServerData() bool

	// ForceControlSubsystems causes the control system to process each system. Regardless of the last hash value.
	SetForceControlSubsystems(force bool) error
	ForceControlSubsystems() bool

	// ControlServerURL URL for control server.
	SetControlServerURL(url string) error
	ControlServerURL() string

	// ControlRequestInterval is the interval at which control client will check for updates from the control server.
	SetControlRequestInterval(interval time.Duration) error
	ControlRequestInterval() time.Duration

	// DisableControlTLS disables TLS transport with the control server.
	SetDisableControlTLS(disabled bool) error
	DisableControlTLS() bool

	// InsecureControlTLS disables TLS certificate validation for the control server.
	SetInsecureControlTLS(disabled bool) error
	InsecureControlTLS() bool
}

// FlagsChangeObserver is an interface to be notified of changes to flags.
type FlagsChangeObserver interface {
	// FlagsChanged tells the observer that flag changes have occurred.
	FlagsChanged(keys ...FlagKey)
}
