package types

// OsqueryInstanceTracker is the interface that we expect knapsack to implement,
// allowing setting and retrieving of the underlying osquery history
type OsqueryInstanceTracker interface {
	OsqueryHistory() OsqueryHistorian
	SetOsqueryHistory(osqHistory OsqueryHistorian)
}

// OsqueryHistorian is the interface that our history.History implements, allowing any paths towards
// reading and writing various parts of our collected history. all instance history manipulation should go through here
type OsqueryHistorian interface {
	NewInstance(registrationId string, runId string) error
	GetHistory() ([]map[string]string, error)
	LatestInstanceStats(registrationId string) (map[string]string, error)
	LatestInstanceId(registrationId string) (string, error)
	LatestInstanceUptimeMinutes(registrationId string) (int64, error)
	SetConnected(runId string, querier Querier) error
	SetExited(runId string, exitError error) error
}

type Querier interface {
	Query(query string) ([]map[string]string, error)
}
