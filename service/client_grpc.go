package service

import (
	"github.com/kolide/agent-api"

	grpctransport "github.com/go-kit/kit/transport/grpc"
	"google.golang.org/grpc"
)

func New(conn *grpc.ClientConn) KolideService {
	requestEnrollmentEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"RequestEnrollment",
		EncodeGRPCEnrollmentRequest,
		DecodeGRPCEnrollmentResponse,
		kolide_agent.EnrollmentResponse{},
	).Endpoint()

	requestConfigEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"RequestConfig",
		EncodeGRPCAgentAPIRequest,
		DecodeGRPCConfigResponse,
		kolide_agent.ConfigResponse{},
	).Endpoint()

	publishLogsEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"PublishLogs",
		EncodeGRPCLogCollection,
		DecodeGRPCAgentAPIResponse,
		kolide_agent.AgentApiResponse{},
	).Endpoint()

	requestQueriesEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"RequestQueries",
		EncodeGRPCAgentAPIRequest,
		DecodeGRPCQueryCollection,
		kolide_agent.QueryCollection{},
	).Endpoint()

	publishResultsEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"PublishResults",
		EncodeGRPCResultCollection,
		DecodeGRPCAgentAPIRequest,
		kolide_agent.AgentApiResponse{},
	).Endpoint()

	return Endpoints{
		RequestEnrollmentEndpoint: requestEnrollmentEndpoint,
		RequestConfigEndpoint:     requestConfigEndpoint,
		PublishLogsEndpoint:       publishLogsEndpoint,
		RequestQueriesEndpoint:    requestQueriesEndpoint,
		PublishResultsEndpoint:    publishResultsEndpoint,
	}
}
