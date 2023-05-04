package types

// Querier is an interface for asynchronously querying osquery.
type Querier interface {
	// Query will attempt to send a query to the osquery client. The result of the
	// query will be passed to the callback function provided.
	Query(query string, callback func(result []map[string]string, err error)) error
}
