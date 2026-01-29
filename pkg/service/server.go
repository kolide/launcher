package service

import (
	"net/http"
	"sync"

	"github.com/go-kit/kit/endpoint"
	"github.com/kolide/launcher/ee/agent/types"
)

// KolideClient is an alias for the Endpoints type.
// It's added to aid in maintaining backwards compatibility for imports.
type KolideClient = Endpoints

// Endpoints defines the endpoints implemented by the Kolide remote extension servers and clients.
type Endpoints struct {
	RequestEnrollmentEndpoint endpoint.Endpoint
	RequestConfigEndpoint     endpoint.Endpoint
	PublishLogsEndpoint       endpoint.Endpoint
	RequestQueriesEndpoint    endpoint.Endpoint
	PublishResultsEndpoint    endpoint.Endpoint
	CheckHealthEndpoint       endpoint.Endpoint
	endpointsLock             *sync.RWMutex // locks in the rare case that we have to update the underlying endpoint URL
	client                    *http.Client
	k                         types.Knapsack
}
