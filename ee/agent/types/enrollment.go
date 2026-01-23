package types

type EnrollmentStatus string

const (
	NoEnrollmentKey EnrollmentStatus = "no_enrollment_key"
	Unenrolled      EnrollmentStatus = "unenrolled"
	Enrolled        EnrollmentStatus = "enrolled"
	Unknown         EnrollmentStatus = "unknown"

	DefaultEnrollmentID = "default"
)

// EnrollmentDetails is the set of details that are collected from osqueryd
// that are sent to the Kolide server during enrollment.
type EnrollmentDetails struct {
	OSVersion                 string `json:"os_version"`
	OSBuildID                 string `json:"os_build_id"`
	OSPlatform                string `json:"os_platform"`
	Hostname                  string `json:"hostname"`
	HardwareVendor            string `json:"hardware_vendor"`
	HardwareModel             string `json:"hardware_model"`
	HardwareSerial            string `json:"hardware_serial"`
	OsqueryVersion            string `json:"osquery_version"`
	LauncherHardwareKey       string `json:"launcher_hardware_key"`
	LauncherHardwareKeySource string `json:"launcher_hardware_key_source"`
	LauncherLocalKey          string `json:"launcher_local_key"`
	LauncherVersion           string `json:"launcher_version"`
	OSName                    string `json:"os_name"`
	OSPlatformLike            string `json:"os_platform_like"`
	GOOS                      string `json:"goos"`
	GOARCH                    string `json:"goarch"`
	HardwareUUID              string `json:"hardware_uuid"`
}

// EnrollmentTracker manages the current set of enrollments for this launcher installation.
// Right now, the list is hardcoded to only the default enrollment ID. In the future, this
// data may be provided by e.g. a control server subsystem.
type EnrollmentTracker interface {
	EnrollmentIDs() []string
	Registrations() ([]Enrollment, error)
	SaveRegistration(registrationId, munemo, nodeKey, enrollmentSecret string) error
	EnsureEnrollmentStored(enrollmentId string) error
	NodeKey(registrationId string) (string, error)
	DeleteEnrollment(enrollmentId string) error
}

// Enrollment represents a launcher installation's association with a given tenant.
// For now, until we tackle the multitenancy project, the enrollment ID is always
// DefaultEnrollmentID, and we expect a launcher installation to have only one enrollment.
type Enrollment struct {
	EnrollmentID     string `json:"registration_id"` // Stored under "registration_id" for legacy reasons/backwards compatibility
	Munemo           string `json:"munemo"`
	EnrollmentSecret string `json:"enrollment_secret,omitempty"`
	NodeKey          string `json:"node_key"`
}
