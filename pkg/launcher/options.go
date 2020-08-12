package launcher

import (
	"time"

	"github.com/kolide/launcher/pkg/autoupdate"
)

// Options is the set of options that may be configured for Launcher.
type Options struct {
	// KolideServerURL is the URL of the management server to connect to.
	KolideServerURL string
	// EnrollSecret contains the raw enroll secret.
	EnrollSecret string
	// EnrollSecretPath contains the path to a file containing the enroll
	// secret.
	EnrollSecretPath string
	// RootDirectory is the directory that should be used as the osquery
	// root directory (database files, pidfile, etc.).
	RootDirectory string
	// OsquerydPath is the path to the osqueryd binary.
	OsquerydPath string
	// CertPins are optional hashes of subject public key info to use for
	// certificate pinning.
	CertPins [][]byte
	// RootPEM is the path to the pem file containing the certificate
	// chain, if necessary for verification.
	RootPEM string
	// LoggingInterval is the interval at which logs should be flushed to
	// the server.
	LoggingInterval time.Duration
	// EnableInitialRunner enables running scheduled queries immediately
	// (before first schedule interval passes).
	EnableInitialRunner bool
	// Transport the transport that should be used for remote
	// communication.
	Transport string

	// Control enables the remote control functionality.
	Control bool
	// ControlServerURL URL for control server.
	ControlServerURL string
	// GetShellsInterval is the interval at which the control server should
	// be checked for shells.
	GetShellsInterval time.Duration

	// Autoupdate enables the autoupdate functionality.
	Autoupdate bool
	// NotaryServerURL is the URL for the Notary server.
	NotaryServerURL string
	// MirrorServerURL is the URL for the Notary mirror.
	MirrorServerURL string
	// AutoupdateInterval is the interval at which Launcher will check for
	// updates.
	AutoupdateInterval time.Duration
	// UpdateChannel is the channel to pull options from (stable, beta, nightly).
	UpdateChannel autoupdate.UpdateChannel
	// NotaryPrefix is the path prefix used to store launcher and osqueryd binaries on the Notary server
	NotaryPrefix string

	// Debug enables debug logging.
	Debug bool
	// Optional file to mirror debug logs to
	DebugLogFile string
	// OsqueryVerbose puts osquery into verbose mode
	OsqueryVerbose bool
	// OsqueryFlags defines additional flags to pass to osquery (possibly
	// overriding Launcher defaults)
	OsqueryFlags []string
	// DisableControlTLS disables TLS transport with the control server.
	DisableControlTLS bool
	// InsecureTLS disables TLS certificate verification.
	InsecureTLS bool
	// InsecureTransport disables TLS in the transport layer.
	InsecureTransport bool
}
