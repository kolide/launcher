package types

const (
	DefaultRegistrationID = "default"
)

// RegistrationTracker manages the current set of registrations for this launcher installation.
// Right now, the list is hardcoded to only the default registration ID. In the future, this
// data may be provided by e.g. a control server subsystem.
type RegistrationTracker interface {
	RegistrationIDs() []string
}
