package types

type Stores interface {
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
