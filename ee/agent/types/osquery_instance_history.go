package types

// OsqueryInstanceTracker is the interface that we'll expect knapsack to implement,
// allowing setting and retrieving of the underlying osquery history
type OsqueryInstanceTracker interface {
	OsqueryHistory() OsqueryHistorian
	SetOsqueryHistory(osqHistory OsqueryHistorian)
}

// OsqueryHistory is the interface that our history.History implements, allowing various paths towards
// reading and writing various parts of our collected history. all instance history manipulation should go through here
type OsqueryHistorian interface {
	NewInstance(registrationId string, runId string) error
	GetHistory() ([]map[string]string, error)
	// LatestInstance() (*OsqueryInstanceStats, error)
	LatestInstanceIDByRegistrationID(registrationId string) (string, error)
	// LatestInstanceByRegistrationID(registrationId string)
	LatestInstanceUptimeMinutes() (int64, error)
	SetConnected(runID string, querier Querier) error
	SetExited(runID string, exitError error) error
}

// TODO move these all to top level history operations, all updates set through there
// OsqueryInstanceStats is the interface that our history.Instance implements, allowing callers to update
// various states throughout the osquery instance management lifecycle
// type OsqueryInstanceStats interface {
// 	Connected(querier Querier) error
// 	Exited(exitError error) error
// }

type Querier interface {
	Query(query string) ([]map[string]string, error)
}
