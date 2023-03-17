package types

// Storage is an interface for accesing the underlying data stores
type Storage interface {
	AgentFlagsStore() KVStore
	AutoupdateErrorsStore() KVStore
	ConfigStore() KVStore
	ControlStore() KVStore
	InitialResultsStore() KVStore
	ResultLogsStore() KVStore
	OsqueryHistoryInstanceStore() KVStore
	SentNotificationsStore() KVStore
	StatusLogsStore() KVStore
	ServerProvidedDataStore() KVStore
}
