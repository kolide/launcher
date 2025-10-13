package types

import (
	"time"

	"github.com/kolide/launcher/ee/agent/flags/keys"
)

// Flags is an interface for setting and retrieving launcher agent flags.
type Flags interface {
	// Registers an observer to receive messages when the specified keys change.
	RegisterChangeObserver(observer FlagsChangeObserver, flagKeys ...keys.FlagKey)
	// Deregisters an existing observer
	DeregisterChangeObserver(observer FlagsChangeObserver)

	// KolideServerURL is the URL of the management server to connect to.
	SetKolideServerURL(url string) error
	KolideServerURL() string

	// KolideHosted true if using Kolide SaaS settings.
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
	SetLoggingInterval(interval time.Duration) error
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

	// DesktopUpdateInterval is the interval on which desktop processes will be spawned, if necessary.
	SetDesktopUpdateInterval(interval time.Duration) error
	DesktopUpdateInterval() time.Duration

	// DesktopMenuRefreshInterval is the interval on which the desktop menu will be refreshed.
	SetDesktopMenuRefreshInterval(interval time.Duration) error
	DesktopMenuRefreshInterval() time.Duration

	// DesktopGoMaxProcs is the maximum number of OS threads that can be used by the desktop process.
	SetDesktopGoMaxProcs(maxProcs int) error
	DesktopGoMaxProcs() int

	// LauncherGoMaxProcs is the maximum number of OS threads that can be used by the launcher process.
	SetLauncherGoMaxProcs(maxProcs int) error
	LauncherGoMaxProcs() int

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
	SetControlRequestIntervalOverride(value time.Duration, duration time.Duration)
	ControlRequestInterval() time.Duration

	// AllowOverlyBroadDt4aAcceleration enables acceleration via /v3/dt4a localserver endpoint. It is a test flag
	// for development use; it should ultimately be replaced by a call to a new /v3 endpoint that only
	// performs acceleration.
	SetAllowOverlyBroadDt4aAcceleration(enable bool) error
	AllowOverlyBroadDt4aAcceleration() bool

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

	// IAmBreakingEELicence disables the EE licence check before running the local server
	IAmBreakingEELicense() bool

	// Debug enables debug logging.
	SetDebug(debug bool) error
	Debug() bool

	// DebugLogFile is an optional file to mirror debug logs to.
	DebugLogFile() string

	// OsqueryVerbose puts osquery into verbose mode.
	SetOsqueryVerbose(verbose bool) error
	OsqueryVerbose() bool

	// DistributedForwardingInterval indicates the rate at which we forward osquery distributed requests
	// to the cloud
	SetDistributedForwardingInterval(interval time.Duration) error
	SetDistributedForwardingIntervalOverride(value time.Duration, duration time.Duration)
	DistributedForwardingInterval() time.Duration

	// WatchdogEnabled enables the osquery watchdog
	SetWatchdogEnabled(enable bool) error
	WatchdogEnabled() bool

	// WatchdogDelaySec sets the number of seconds the watchdog will delay on startup before running
	SetWatchdogDelaySec(sec int) error
	WatchdogDelaySec() int

	// WatchdogMemoryLimitMB sets the memory limit on osquery processes
	SetWatchdogMemoryLimitMB(limit int) error
	WatchdogMemoryLimitMB() int

	// WatchdogUtilizationLimitPercent sets the CPU utilization limit on osquery processes
	SetWatchdogUtilizationLimitPercent(limit int) error
	WatchdogUtilizationLimitPercent() int

	// OsqueryFlags defines additional flags to pass to osquery (possibly
	// overriding Launcher defaults)
	OsqueryFlags() []string

	// Osquery Version is the version of osquery that is being used.
	SetCurrentRunningOsqueryVersion(version string) error
	CurrentRunningOsqueryVersion() string

	// Autoupdate enables the autoupdate functionality.
	SetAutoupdate(enabled bool) error
	Autoupdate() bool

	// TufServerURL is the URL for the tuf server.
	SetTufServerURL(url string) error
	TufServerURL() string

	// MirrorServerURL is the URL for the TUF mirror.
	SetMirrorServerURL(url string) error
	MirrorServerURL() string

	// AutoupdateInterval is the interval at which Launcher will check for updates.
	SetAutoupdateInterval(interval time.Duration) error
	AutoupdateInterval() time.Duration

	// UpdateChannel is the channel to pull options from (stable, beta, nightly).
	SetUpdateChannel(channel string) error
	UpdateChannel() string

	// AutoupdateInitialDelay set an initial startup delay on the autoupdater process.
	SetAutoupdateInitialDelay(delay time.Duration) error
	AutoupdateInitialDelay() time.Duration

	// UpdateDirectory is the location of the update libraries for osqueryd and launcher
	SetUpdateDirectory(directory string) error
	UpdateDirectory() string

	// PinnedLauncherVersion is the launcher version to lock the autoupdater to, rather than autoupdating via the update channel.
	SetPinnedLauncherVersion(version string) error
	PinnedLauncherVersion() string

	// PinnedOsquerydVersion is the osqueryd version to lock the autoupdater to, rather than autoupdating via the update channel.
	SetPinnedOsquerydVersion(version string) error
	PinnedOsquerydVersion() string

	// ExportTraces enables exporting our traces
	SetExportTraces(enabled bool) error
	SetExportTracesOverride(value bool, duration time.Duration)
	ExportTraces() bool

	// TraceSamplingRate is a number between 0.0 and 1.0 that indicates what fraction of traces should be sampled.
	SetTraceSamplingRate(rate float64) error
	SetTraceSamplingRateOverride(value float64, duration time.Duration)
	TraceSamplingRate() float64

	// LogIngestServerURL is the URL of the ingest server for logs
	SetLogIngestServerURL(url string) error
	LogIngestServerURL() string

	// LogShippingLevel is the level at which logs should be shipped to the server
	SetLogShippingLevel(level string) error
	SetLogShippingLevelOverride(value string, duration time.Duration)
	LogShippingLevel() string

	// TraceIngestServerURL is the URL of the ingest server for traces
	SetTraceIngestServerURL(url string) error
	TraceIngestServerURL() string

	// DisableTraceIngestTLS disables TLS for observability ingest server communication
	SetDisableTraceIngestTLS(enabled bool) error
	DisableTraceIngestTLS() bool

	// TraceBatchTimeout is the maximum amount of time before the trace exporter will export the next batch of spans
	SetTraceBatchTimeout(duration time.Duration) error
	TraceBatchTimeout() time.Duration

	// InModernStandby indicates whether a Windows machine is awake or in modern standby
	SetInModernStandby(enabled bool) error
	InModernStandby() bool

	// OsqueryHealthcheckStartupDelay is the time to wait before beginning osquery healthchecks
	SetOsqueryHealthcheckStartupDelay(delay time.Duration) error
	OsqueryHealthcheckStartupDelay() time.Duration

	// LocalDevelopmentPath points to a local build of launcher to use instead of the one selected from the autoupdate library
	LocalDevelopmentPath() string

	// LauncherWatchdogEnabled controls whether launcher installs/runs, or stops/removes the launcher watchdog service
	SetLauncherWatchdogEnabled(enabled bool) error
	LauncherWatchdogEnabled() bool

	// SystrayRestartEnabled controls whether launcher's desktop runner will restart systray on error
	SetSystrayRestartEnabled(enabled bool) error
	SystrayRestartEnabled() bool

	// Identifier is the package build identifier used to namespace our paths and service names
	Identifier() string

	// TableGenerateTimeout is the maximum time a Kolide extension table is permitted to take
	SetTableGenerateTimeout(interval time.Duration) error
	TableGenerateTimeout() time.Duration

	// UseCachedDataForScheduledQueries controls whether launcher uses cached data for scheduled queries.
	// Currently, we do this only for the kolide_windows_updates table, since that table can time out when
	// querying for fresh data.
	SetUseCachedDataForScheduledQueries(enabled bool) error
	UseCachedDataForScheduledQueries() bool

	// CachedQueryResultsTTL indicates how long cached query results are valid.
	SetCachedQueryResultsTTL(ttl time.Duration) error
	CachedQueryResultsTTL() time.Duration

	// ResetOnHardwareChangeEnabled controls whether launcher will reset its database on hardware change detected
	SetResetOnHardwareChangeEnabled(enabled bool) error
	ResetOnHardwareChangeEnabled() bool

	AutoupdateDownloadSplay() time.Duration
	SetAutoupdateDownloadSplay(val time.Duration) error

	// PerformanceMonitoringEnabled controls whether launcher self-monitors for performance issues
	SetPerformanceMonitoringEnabled(enabled bool) error
	PerformanceMonitoringEnabled() bool

	// DuplicateLogWindow is the time window for deduplicating duplicate log records
	SetDuplicateLogWindow(duration time.Duration) error
	DuplicateLogWindow() time.Duration
}
