package keys

// FlagKeys are named identifiers corresponding to flags
type FlagKey string

// When adding a new FlagKey:
// 1. Define the FlagKey identifier, and the string key value it corresponds to, in the block below
// 2. Add a getter and setter to the Flags interface (flags.go)
// 3. Implement the getter and setter in the Knapsack, which delegates the call to the FlagController
// 4. Implement the getter and setter in the FlagController, providing defaults, limits, and overrides
// 4. Implement tests for any new APIs, sanitizers, limits, overrides.
// 5. Update mocks -- in pkg/agent/types, run `mockery --name Knapsack` and `mockery --name Flags`.
const (
	KolideServerURL                FlagKey = "hostname"
	KolideHosted                   FlagKey = "kolide_hosted"
	Transport                      FlagKey = "transport"
	LoggingInterval                FlagKey = "logging_interval"
	OsquerydPath                   FlagKey = "osqueryd_path"
	OsqueryHealthcheckStartupDelay FlagKey = "osquery_healthcheck_startup_delay"
	RootDirectory                  FlagKey = "root_directory"
	RootPEM                        FlagKey = "root_pem"
	DesktopEnabled                 FlagKey = "desktop_enabled_v1"
	DesktopUpdateInterval          FlagKey = "desktop_update_interval"
	DesktopMenuRefreshInterval     FlagKey = "desktop_menu_refresh_interval"
	DebugServerData                FlagKey = "debug_server_data"
	ForceControlSubsystems         FlagKey = "force_control_subsystems"
	ControlServerURL               FlagKey = "control_server_url"
	ControlRequestInterval         FlagKey = "control_request_interval"
	DisableControlTLS              FlagKey = "disable_control_tls"
	InsecureControlTLS             FlagKey = "insecure_control_tls"
	InsecureTLS                    FlagKey = "insecure_tls"
	InsecureTransportTLS           FlagKey = "insecure_transport"
	IAmBreakingEELicense           FlagKey = "i-am-breaking-ee-license"
	Debug                          FlagKey = "debug"
	DebugLogFile                   FlagKey = "debug_log_file"
	OsqueryVerbose                 FlagKey = "osquery_verbose"
	Autoupdate                     FlagKey = "autoupdate"
	NotaryServerURL                FlagKey = "notary_url"
	TufServerURL                   FlagKey = "tuf_url"
	MirrorServerURL                FlagKey = "mirror_url"
	AutoupdateInterval             FlagKey = "autoupdate_interval"
	UpdateChannel                  FlagKey = "update_channel"
	NotaryPrefix                   FlagKey = "notary_prefix"
	AutoupdateInitialDelay         FlagKey = "autoupdater_initial_delay"
	UpdateDirectory                FlagKey = "update_directory"
	ExportTraces                   FlagKey = "export_traces"
	TraceSamplingRate              FlagKey = "trace_sampling_rate"
	LogIngestServerURL             FlagKey = "log_ingest_url"
	TraceIngestServerURL           FlagKey = "trace_ingest_url"
	DisableTraceIngestTLS          FlagKey = "disable_trace_ingest_tls"
	InModernStandby                FlagKey = "in_modern_standby"
)

func (key FlagKey) String() string {
	return string(key)
}

func ToFlagKeys(s []string) []FlagKey {
	f := make([]FlagKey, len(s))
	for i, v := range s {
		f[i] = FlagKey(v)
	}
	return f
}

// Returns the intersection of FlagKeys; keys which exist in both a and b.
func Intersection(a, b []FlagKey) []FlagKey {
	m := make(map[FlagKey]bool)
	var result []FlagKey

	for _, element := range a {
		m[element] = true
	}

	for _, element := range b {
		if m[element] {
			result = append(result, element)
		}
	}

	return result
}
