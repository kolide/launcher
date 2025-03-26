package types

import "github.com/kolide/launcher/ee/agent/storage"

type Stores interface {
	Stores() map[storage.Store]KVStore
	AgentFlagsStore() KVStore
	KatcConfigStore() KVStore
	AutoupdateErrorsStore() KVStore
	ConfigStore() KVStore
	ControlStore() KVStore
	PersistentHostDataStore() KVStore
	InitialResultsStore() KVStore
	ResultLogsStore() KVStore
	OsqueryHistoryInstanceStore() KVStore
	SentNotificationsStore() KVStore
	StatusLogsStore() KVStore
	ServerProvidedDataStore() KVStore
	TokenStore() KVStore
	LauncherHistoryStore() KVStore
	Dt4aInfoStore() KVStore
}
