package types

// Stores are named identifiers
type Store string

const (
	AgentFlagsStore         Store = "agent_flags"
	ConfigStore             Store = "config" // Bucket name to use for launcher configuration.
	ControlStore            Store = "control_service_data"
	InitialResultsStore     Store = "initial_results"
	ResultLogsStore         Store = "result_logs" // Bucket name to use for buffered result logs.
	OsqueryHistoryInstance  Store = "osquery_instance_history"
	SentNotificationsStore  Store = "sent_notifications"   // The bucket where we hold sent notifications.
	StatusLogsStore         Store = "status_logs"          // Bucket name to use for buffered status logs.
	ServerProvidedDataStore Store = "server_provided_data" // The bucket which we push values into from server-backed tables.
)

func (storeType Store) String() string {
	return string(storeType)
}

// Storage is an interface for accesing the underlying data stores
type Storage interface {
	GetStore(storeType Store) KVStore
}
