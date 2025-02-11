package types

// OsqueryInstanceTracker is the interface that we'll expect knapsack to implement,
// allowing setting and retrieving of the underlying osquery history
type OsqueryInstanceTracker interface {
	OsqueryHistory() OsqueryHistorian
	SetOsqueryHistory(osqHistory OsqueryHistorian)
}

// OsqueryHistory is the interface that our history.History implements, allowing any paths towards
// reading and writing various parts of our collected history. all instance history manipulation should go through here
type OsqueryHistorian interface {
	NewInstance(registrationId string, runId string) error
	GetHistory() ([]map[string]string, error)
	LatestInstanceIDByRegistrationID(registrationId string) (string, error)
	LatestInstanceUptimeMinutes() (int64, error)
	SetConnected(runID string, querier Querier) error
	SetExited(runID string, exitError error) error
}

type Querier interface {
	Query(query string) ([]map[string]string, error)
}
