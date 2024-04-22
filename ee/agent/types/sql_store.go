package types

import (
	"context"
)

// ResultFetcher is an interface for querying a single text field from a structured data store.
// This is intentionally vague for potential future re-use, allowing the caller to unmarshal string results as needed.
// This interface is compatible with any tables that include a timestamp, and any text field of interest
type ResultFetcher interface {
	// Fetch retrieves all rows provided by the results of executing query
	FetchResults(ctx context.Context, columnName string) ([][]byte, error)
	// FetchLatest retrieves the most recent value for columnName
	FetchLatestResult(ctx context.Context, columnName string) ([]byte, error)
	Closer
}

type ResultSetter interface {
	// AddResult marshals
	AddResult(ctx context.Context, timestamp int64, result []byte) error
	Closer
}

// TODO add rotation interface to cap limit on health check results
