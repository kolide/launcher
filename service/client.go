package service

import (
	"github.com/go-kit/kit/endpoint"
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
}

type agentAPIRequest struct {
	NodeKey string
}

type agentAPIResponse struct {
	Message     string
	ErrorCode   string
	NodeInvalid bool
	Err         error
}

func MakeServerEndpoints(svc KolideService) Endpoints {
	return Endpoints{
		RequestEnrollmentEndpoint: MakeRequestEnrollmentEndpoint(svc),
		RequestConfigEndpoint:     MakeRequestConfigEndpoint(svc),
		PublishLogsEndpoint:       MakePublishLogsEndpoint(svc),
		RequestQueriesEndpoint:    MakeRequestQueriesEndpoint(svc),
		PublishResultsEndpoint:    MakePublishResultsEndpoint(svc),
		CheckHealthEndpoint:       MakeCheckHealthEndpoint(svc),
	}
}
