package types

type EnrollmentStatus string

const (
	NoEnrollmentKey EnrollmentStatus = "no_enrollment_key"
	Unenrolled      EnrollmentStatus = "unenrolled"
	Enrolled        EnrollmentStatus = "enrolled"
	Unknown         EnrollmentStatus = "unknown"
)

// Move EnrollmentDetails from service package to types
type EnrollmentDetails struct {
	OSVersion                 string
	OSBuildID                 string
	OSPlatform                string
	Hostname                  string
	HardwareVendor            string
	HardwareModel             string
	HardwareSerial            string
	OsqueryVersion            string
	LauncherHardwareKey       string
	LauncherHardwareKeySource string
	LauncherLocalKey          string
	LauncherVersion           string
	OSName                    string
	OSPlatformLike            string
	GOOS                      string
	GOARCH                    string
	HardwareUUID              string
}
