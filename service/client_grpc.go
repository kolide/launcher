package service

import (
	"context"

	"github.com/kolide/agent-api"
	"github.com/kolide/launcher/service/uuid"

	"github.com/go-kit/kit/log"
	grpctransport "github.com/go-kit/kit/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func attachUUID() grpctransport.ClientOption {
	return grpctransport.ClientBefore(
		func(ctx context.Context, md *metadata.MD) context.Context {
			uuid, _ := uuid.FromContext(ctx)
			return grpctransport.SetRequestHeader("uuid", uuid)(ctx, md)
		},
	)
}

// New creates a new KolideClient (implementation of the KolideService
// interface) using the provided gRPC client connection.
func New(conn *grpc.ClientConn, logger log.Logger) KolideService {
	requestEnrollmentEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"RequestEnrollment",
		encodeGRPCEnrollmentRequest,
		decodeGRPCEnrollmentResponse,
		kolide_agent.EnrollmentResponse{},
		attachUUID(),
	).Endpoint()

	requestConfigEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"RequestConfig",
		encodeGRPCAgentAPIRequest,
		decodeGRPCConfigResponse,
		kolide_agent.ConfigResponse{},
		attachUUID(),
	).Endpoint()

	publishLogsEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"PublishLogs",
		encodeGRPCLogCollection,
		decodeGRPCAgentAPIResponse,
		kolide_agent.AgentApiResponse{},
		attachUUID(),
	).Endpoint()

	requestQueriesEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"RequestQueries",
		encodeGRPCAgentAPIRequest,
		decodeGRPCQueryCollection,
		kolide_agent.QueryCollection{},
		attachUUID(),
	).Endpoint()

	publishResultsEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"PublishResults",
		encodeGRPCResultCollection,
		decodeGRPCAgentAPIResponse,
		kolide_agent.AgentApiResponse{},
		attachUUID(),
	).Endpoint()

	var client KolideService = KolideClient{
		RequestEnrollmentEndpoint: requestEnrollmentEndpoint,
		RequestConfigEndpoint:     requestConfigEndpoint,
		PublishLogsEndpoint:       publishLogsEndpoint,
		RequestQueriesEndpoint:    requestQueriesEndpoint,
		PublishResultsEndpoint:    publishResultsEndpoint,
	}
	client = loggingMiddleware(logger)(client)
	// Wrap with UUID middleware after logger so that UUID is available in
	// the logger context.
	client = uuidMiddleware(client)

	return client
}
