package types

type EnrollmentStatus string

const (
	NoEnrollmentKey EnrollmentStatus = "no_enrollment_key"
	Unenrolled      EnrollmentStatus = "unenrolled"
	Enrolled        EnrollmentStatus = "enrolled"
	Unknown         EnrollmentStatus = "unknown"
)
