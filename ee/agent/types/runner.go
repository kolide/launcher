package types

type (
	// RegistrationChangeHandler is implemented by pkg/osquery/runtime/runner.go
	RegistrationChangeHandler interface {
		UpdateRegistrationIDs(registrationIDs []string) error
	}

	OsqRunner interface {
		RegistrationChangeHandler
		InstanceQuerier
	}
)
