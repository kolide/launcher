// Package service defines the interface used by the launcher to communicate
// with the Kolide server. It currently only uses the gRPC transport, but could
// be extended to use others.
package service

import (
	"context"

	"github.com/kolide/osquery-go/plugin/distributed"
	"github.com/kolide/osquery-go/plugin/logger"
)

// KolideService is the interface exposed by the Kolide server.
type KolideService interface {
	// RequestEnrollment requests a node key for the host, authenticating
	// with the given enroll secret.
	RequestEnrollment(ctx context.Context, enrollSecret, hostIdentifier string) (string, bool, error)
	// RequestConfig requests the osquery config for the host.
	RequestConfig(ctx context.Context, nodeKey string) (string, bool, error)
	// RequestPractices requests a set of query to broadcast the results of locally
	RequestPractices(ctx context.Context, nodeKey string) (*distributed.GetQueriesResult, bool, error)
	// PublishLogs publishes logs from the osquery process. These may be
	// status logs or result logs from scheduled queries.
	PublishLogs(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error)
	// RequestQueries requests the distributed queries to execute.
	RequestQueries(ctx context.Context, nodeKey string) (*distributed.GetQueriesResult, bool, error)
	// PublishResults publishes the results of executed distributed queries.
	PublishResults(ctx context.Context, nodeKey string, results []distributed.Result) (string, string, bool, error)
	// CheckHealth returns the status of the remote API, with 1 indicating OK status.
	CheckHealth(ctx context.Context) (int32, error)
}
