package types

import (
	"time"

	"github.com/kolide/launcher/pkg/agent/flags/keys"
)

// Flags is an interface for setting and retrieving launcher agent flags.
type Flags interface {
	// Registers an observer to receive messages when the specified keys change.
	RegisterChangeObserver(observer FlagsChangeObserver, flagKeys ...keys.FlagKey)

	// KolideServerURL is the URL of the management server to connect to.
	SetKolideServerURL(url string) error
	KolideServerURL() string

	// KolideHosted true if using Kolide SaaS settings.
	SetKolideHosted(hosted bool) error
	KolideHosted() bool

	// EnrollSecret contains the raw enroll secret.
	EnrollSecret() string

	// EnrollSecretPath contains the path to a file containing the enroll
	// secret.
	EnrollSecretPath() string

	// RootDirectory is the directory that should be used as the osquery
	// root directory (database files, pidfile, etc.).
	RootDirectory() string

	// OsquerydPath is the path to the osqueryd binary.
	OsquerydPath() string

	// CertPins are optional hashes of subject public key info to use for
	// certificate pinning.
	CertPins() [][]byte

	// RootPEM is the path to the pem file containing the certificate
	// chain, if necessary for verification.
	RootPEM() string

	// LoggingInterval is the interval at which logs should be flushed to
	// the server.
	LoggingInterval() time.Duration

	// EnableInitialRunner enables running scheduled queries immediately
	// (before first schedule interval passes).
	EnableInitialRunner() bool

	// Transport the transport that should be used for remote
	// communication.
	Transport() string

	// LogMaxBytesPerBatch sets the maximum bytes allowed in a batch
	// of log. When blank, launcher will pick a value
	// appropriate for the transport.
	LogMaxBytesPerBatch() int

	// DesktopEnabled causes the launcher desktop process and GUI to be enabled.
	SetDesktopEnabled(enabled bool) error
	DesktopEnabled() bool

	// DebugServerData causes logging and diagnostics related to control server error handling to be enabled.
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
	// SetControlRequestIntervalOverride stores an interval to be temporarily used as an override of any other interval, until the duration has elapased.
	SetControlRequestIntervalOverride(interval, duration time.Duration)
	ControlRequestInterval() time.Duration

	// DisableControlTLS disables TLS transport with the control server.
	SetDisableControlTLS(disabled bool) error
	DisableControlTLS() bool

	// InsecureControlTLS disables TLS certificate validation for the control server.
	SetInsecureControlTLS(disabled bool) error
	InsecureControlTLS() bool

	// InsecureTLS disables TLS certificate verification.
	SetInsecureTLS(insecure bool) error
	InsecureTLS() bool

	// InsecureTransport disables TLS in the transport layer.
	SetInsecureTransportTLS(insecure bool) error
	InsecureTransportTLS() bool

	// CompactDbMaxTx sets the max transaction size for bolt db compaction operations
	SetCompactDbMaxTx(max int64) error
	CompactDbMaxTx() int64

	// IAmBreakingEELicence disables the EE licence check before running the local server
	SetIAmBreakingEELicense(disabled bool) error
	IAmBreakingEELicense() bool

	// Debug enables debug logging.
	SetDebug(debug bool) error
	Debug() bool

	// DebugLogFile is an optional file to mirror debug logs to.
	SetDebugLogFile(file string) error
	DebugLogFile() string

	// OsqueryVerbose puts osquery into verbose mode.
	SetOsqueryVerbose(verbose bool) error
	OsqueryVerbose() bool

	// Autoupdate enables the autoupdate functionality.
	SetAutoupdate(enabled bool) error
	Autoupdate() bool

	// NotaryServerURL is the URL for the Notary server.
	SetNotaryServerURL(url string) error
	NotaryServerURL() string

	// TufServerURL is the URL for the tuf server.
	SetTufServerURL(url string) error
	TufServerURL() string

	// MirrorServerURL is the URL for the Notary mirror.
	SetMirrorServerURL(url string) error
	MirrorServerURL() string

	// AutoupdateInterval is the interval at which Launcher will check for updates.
	SetAutoupdateInterval(interval time.Duration) error
	AutoupdateInterval() time.Duration

	// UpdateChannel is the channel to pull options from (stable, beta, nightly).
	SetUpdateChannel(channel string) error
	UpdateChannel() string

	// NotaryPrefix is the path prefix used to store launcher and osqueryd binaries on the Notary server
	SetNotaryPrefix(prefix string) error
	NotaryPrefix() string

	// AutoupdateInitialDelay set an initial startup delay on the autoupdater process.
	SetAutoupdateInitialDelay(delay time.Duration) error
	AutoupdateInitialDelay() time.Duration

	// UpdateDirectory is the location of the update libraries for osqueryd and launcher
	SetUpdateDirectory(directory string) error
	UpdateDirectory() string
}
