package types

import "context"

// Knapsack is an inventory of data and useful services which are used throughout
// launcher code and are typically valid for the lifetime of the launcher application instance.
type Knapsack interface {
	Stores
	BboltDB
	Flags
	Slogger
	RegistrationTracker
	InstanceQuerier
	OsqueryInstanceTracker
	SetInstanceQuerier(q InstanceQuerier)
	// LatestOsquerydPath finds the path to the latest osqueryd binary, after accounting for updates.
	LatestOsquerydPath(ctx context.Context) string
	// ReadEnrollSecret returns the enroll secret value, checking in various locations.
	ReadEnrollSecret() (string, error)
	// CurrentEnrollmentStatus returns the current enrollment status of the launcher installation
	CurrentEnrollmentStatus() (EnrollmentStatus, error)
	// GetRunID returns the current launcher run ID
	GetRunID() string
	// GetEnrollmentDetails returns the enrollment details for the launcher installation
	GetEnrollmentDetails() EnrollmentDetails
	// SetEnrollmentDetails sets the enrollment details for the launcher installation
	SetEnrollmentDetails(details EnrollmentDetails)
}
