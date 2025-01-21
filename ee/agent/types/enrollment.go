package types

type EnrollmentStatus string

const (
	NoEnrollmentKey EnrollmentStatus = "no_enrollment_key"
	Unenrolled      EnrollmentStatus = "unenrolled"
	Enrolled        EnrollmentStatus = "enrolled"
	Unknown         EnrollmentStatus = "unknown"
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
