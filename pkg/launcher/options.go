package launcher

import (
	"runtime"
	"strings"
	"time"

	"github.com/kolide/launcher/pkg/autoupdate"
)

// Options is the set of options that may be configured for Launcher.
type Options struct {
	// AutoloadedExtensions to load with osquery, expected to be in same
	// directory as launcher binary.
	AutoloadedExtensions []string
	// KolideServerURL is the URL of the management server to connect to.
	KolideServerURL string
	// KolideHosted true if using Kolide SaaS settings
	KolideHosted bool
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
	// OsqueryHealthcheckStartupDelay is the time to wait before beginning osquery healthchecks
	OsqueryHealthcheckStartupDelay time.Duration
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
	// LogMaxBytesPerBatch sets the maximum bytes allowed in a batch
	// of log. When blank, launcher will pick a value
	// appropriate for the transport.
	LogMaxBytesPerBatch int

	// Control enables the remote control functionality. It is not in use.
	Control bool
	// ControlServerURL URL for control server.
	ControlServerURL string
	// ControlRequestInterval is the interval at which control client
	// will check for updates from the control server.
	ControlRequestInterval time.Duration

	// Osquery TLS options
	OsqueryTlsConfigEndpoint           string
	OsqueryTlsEnrollEndpoint           string
	OsqueryTlsLoggerEndpoint           string
	OsqueryTlsDistributedReadEndpoint  string
	OsqueryTlsDistributedWriteEndpoint string

	// Autoupdate enables the autoupdate functionality.
	Autoupdate bool
	// NotaryServerURL is the URL for the Notary server.
	NotaryServerURL string
	// TufServerURL is the URL for the tuf server.
	TufServerURL string
	// MirrorServerURL is the URL for the Notary mirror.
	MirrorServerURL string
	// AutoupdateInterval is the interval at which Launcher will check for
	// updates.
	AutoupdateInterval time.Duration
	// UpdateChannel is the channel to pull options from (stable, beta, nightly).
	UpdateChannel autoupdate.UpdateChannel
	// NotaryPrefix is the path prefix used to store launcher and osqueryd binaries on the Notary server
	NotaryPrefix string
	// AutoupdateInitialDelay set an initial startup delay on the autoupdater process.
	AutoupdateInitialDelay time.Duration
	// UpdateDirectory is the location of the update libraries for osqueryd and launcher
	UpdateDirectory string

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
	// InsecureControlTLS disables TLS certificate validation for the control server.
	InsecureControlTLS bool
	// InsecureTLS disables TLS certificate verification.
	InsecureTLS bool
	// InsecureTransport disables TLS in the transport layer.
	InsecureTransport bool
	// CompactDbMaxTx sets the max transaction size for bolt db compaction operations
	CompactDbMaxTx int64
	// IAmBreakingEELicence disables the EE licence check before running the local server
	IAmBreakingEELicense bool
	// DelayStart allows for delaying launcher startup for a configurable amount of time
	DelayStart time.Duration
	// ExportTraces enables exporting traces.
	ExportTraces bool
	// TraceSamplingRate is a number between 0.0 and 1.0 that indicates what fraction of traces should be sampled.
	TraceSamplingRate float64
	// LogIngestServerURL is the URL that logs and other observability data will be exported to
	LogIngestServerURL string
	// TraceIngestServerURL is the URL that traces will be exported to
	TraceIngestServerURL string
	// DisableTraceIngestTLS allows for disabling TLS when connecting to the observability ingest server
	DisableTraceIngestTLS bool

	// ConfigFilePath is the config file options were parsed from, if provided
	ConfigFilePath string

	// LocalDevelopmentPath is the path to a local build of launcher to test against, rather than finding the latest version in the library
	LocalDevelopmentPath string
}

// ConfigFilePath returns the path to launcher's launcher.flags file. If the path
// is available in the command-line args, it will return that path; otherwise, it
// will fall back to a well-known location.
func ConfigFilePath(args []string) string {
	for i, arg := range args {
		if arg == "--config" || arg == "-config" {
			return strings.Trim(args[i+1], `"'`)
		}

		if strings.Contains(arg, "config=") {
			parts := strings.Split(arg, "=")
			if len(parts) == 2 {
				return parts[1]
			}
		}
	}

	// Not found in command-line arguments -- return well-known location instead
	switch runtime.GOOS {
	case "darwin", "linux":
		return "/etc/kolide-k2/launcher.flags"
	case "windows":
		return `C:\Program Files\Kolide\Launcher-kolide-k2\conf\launcher.flags`
	default:
		return ""
	}
}
