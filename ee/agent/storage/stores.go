package storage

// Stores are named identifiers corresponding to key-value buckets
type Store string

const (
	AgentFlagsStore             Store = "agent_flags"              // The store used for agent control flags.
	KatcConfigStore             Store = "katc_config"              // The store used for Kolide custom ATC configuration
	ConfigStore                 Store = "config"                   // The store used for launcher configuration.
	ControlStore                Store = "control_service_data"     // The store used for control service caching data.
	PersistentHostDataStore     Store = "persistent_host_data"     // The store used for data about this host.
	InitialResultsStore         Store = "initial_results"          // The store used for initial runner queries.
	ResultLogsStore             Store = "result_logs"              // The store used for buffered result logs.
	OsqueryHistoryInstanceStore Store = "osquery_instance_history" // The store used for the history of osquery instances.
	SentNotificationsStore      Store = "sent_notifications"       // The store used for sent notifications.
	StatusLogsStore             Store = "status_logs"              // The store used for buffered status logs.
	ServerProvidedDataStore     Store = "server_provided_data"     // The store used for pushing values from server-backed tables.
	TokenStore                  Store = "token_store"              // The store used for holding bearer auth tokens, e.g. the ones used to authenticate with the observability ingest server.
	ControlServerActionsStore   Store = "action_store"             // The store used for storing actions sent by control server.
	LauncherHistoryStore        Store = "launcher_history"         // The store used for storing launcher start time history currently.
	Dt4aInfoStore               Store = "zta_info"                 // The store used for storing dt4a info about this device
	WindowsUpdatesCacheStore    Store = "windows_updates_cache"    // The store used for caching the results of recent searches against the Windows Update Agent API
	RegistrationStore           Store = "registrations"            // The store used for persisting launcher's enrollments/registrations
)

func (storeType Store) String() string {
	return string(storeType)
}
