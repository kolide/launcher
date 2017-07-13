package service

import (
	"github.com/kolide/agent-api"

	grpctransport "github.com/go-kit/kit/transport/grpc"
	"google.golang.org/grpc"
)

// New creates a new KolideClient (implementation of the KolideService
// interface) using the provided gRPC client connection.
func New(conn *grpc.ClientConn) KolideClient {
	requestEnrollmentEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"RequestEnrollment",
		encodeGRPCEnrollmentRequest,
		decodeGRPCEnrollmentResponse,
		kolide_agent.EnrollmentResponse{},
	).Endpoint()

	requestConfigEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"RequestConfig",
		encodeGRPCAgentAPIRequest,
		decodeGRPCConfigResponse,
		kolide_agent.ConfigResponse{},
	).Endpoint()

	publishLogsEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"PublishLogs",
		encodeGRPCLogCollection,
		decodeGRPCAgentAPIResponse,
		kolide_agent.AgentApiResponse{},
	).Endpoint()

	requestQueriesEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"RequestQueries",
		encodeGRPCAgentAPIRequest,
		decodeGRPCQueryCollection,
		kolide_agent.QueryCollection{},
	).Endpoint()

	publishResultsEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"PublishResults",
		encodeGRPCResultCollection,
		decodeGRPCAgentAPIResponse,
		kolide_agent.AgentApiResponse{},
	).Endpoint()

	return KolideClient{
		RequestEnrollmentEndpoint: requestEnrollmentEndpoint,
		RequestConfigEndpoint:     requestConfigEndpoint,
		PublishLogsEndpoint:       publishLogsEndpoint,
		RequestQueriesEndpoint:    requestQueriesEndpoint,
		PublishResultsEndpoint:    publishResultsEndpoint,
	}
}
