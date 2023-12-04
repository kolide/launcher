package types

import "github.com/kolide/launcher/pkg/agent/storage"

type Stores interface {
	Stores() map[storage.Store]KVStore
	AgentFlagsStore() KVStore
	AutoupdateErrorsStore() KVStore
	ConfigStore() KVStore
	ControlStore() KVStore
	HostDataStore() KVStore
	InitialResultsStore() KVStore
	ResultLogsStore() KVStore
	OsqueryHistoryInstanceStore() KVStore
	SentNotificationsStore() KVStore
	StatusLogsStore() KVStore
	ServerProvidedDataStore() KVStore
	TokenStore() KVStore
}
