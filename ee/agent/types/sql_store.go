package types

import (
	"context"
)

// ResultFetcher is an interface for querying a single text field from a structured data store.
// This is intentionally vague for potential future re-use, allowing the caller to unmarshal string results as needed.
// This was initially intended to support the sqlite health_check_results table
type ResultFetcher interface {
	// Fetch retrieves all results rows
	FetchResults(ctx context.Context) ([][]byte, error)
	// FetchLatest retrieves the most recent result based on timestamp column
	FetchLatestResult(ctx context.Context) ([]byte, error)
	Closer
}

type ResultSetter interface {
	// AddResult persists a marshalled result entry alongside the provided unix timestamp
	AddResult(ctx context.Context, timestamp int64, result []byte) error
	Closer
}

// TODO add rotation interface to cap limit on health check results
