package types

const (
	DefaultRegistrationID = "default"
)

// RegistrationTracker manages the current set of registrations for this launcher installation.
// Right now, the list is hardcoded to only the default registration ID. In the future, this
// data may be provided by e.g. a control server subsystem.
type RegistrationTracker interface {
	RegistrationIDs() []string
	Registrations() ([]Registration, error)
}

// Registration represents a launcher installation's association with a given tenant.
// For now, until we tackle the multitenancy project, the registration ID is always
// DefaultRegistrationID, and we expect a launcher installation to have only one registration.
type Registration struct {
	RegistrationID   string `json:"registration_id"`
	Munemo           string `json:"munemo"`
	EnrollmentSecret string `json:"enrollment_secret,omitempty"`
	NodeKey          string `json:"node_key"`
}
