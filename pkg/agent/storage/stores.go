package storage

// Stores are named identifiers corresponding to key-value buckets
type Store string

const (
	AgentFlagsStore             Store = "agent_flags"              // The store used for agent control flags.
	AutoupdateErrorsStore       Store = "tuf_autoupdate_errors"    // The store used for tracking new autoupdater errors.
	ConfigStore                 Store = "config"                   // The store used for launcher configuration.
	ControlStore                Store = "control_service_data"     // The store used for control service caching data.
	InitialResultsStore         Store = "initial_results"          // The store used for initial runner queries.
	ResultLogsStore             Store = "result_logs"              // The store used for buffered result logs.
	OsqueryHistoryInstanceStore Store = "osquery_instance_history" // The store used for the history of osquery instances.
	SentNotificationsStore      Store = "sent_notifications"       // The store used for sent notifications.
	StatusLogsStore             Store = "status_logs"              // The store used for buffered status logs.
	ServerProvidedDataStore     Store = "server_provided_data"     // The store used for pushing values from server-backed tables.
)

func (storeType Store) String() string {
	return string(storeType)
}
