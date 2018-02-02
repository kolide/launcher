package service

import (
	"context"

	"github.com/go-kit/kit/log"
	grpctransport "github.com/go-kit/kit/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/kolide/launcher/service/internal/launcherproto"
	"github.com/kolide/launcher/service/uuid"
)

func attachUUIDHeaderGRPC() grpctransport.ClientOption {
	return grpctransport.ClientBefore(
		func(ctx context.Context, md *metadata.MD) context.Context {
			uuid, _ := uuid.FromContext(ctx)
			return grpctransport.SetRequestHeader("uuid", uuid)(ctx, md)
		},
	)
}

// New creates a new KolideClient (implementation of the KolideService
// interface) using the provided gRPC client connection.
func NewGRPCClient(conn *grpc.ClientConn, logger log.Logger) KolideService {
	requestEnrollmentEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"RequestEnrollment",
		encodeProtobufEnrollmentRequest,
		decodeProtobufEnrollmentResponse,
		kolide_agent.EnrollmentResponse{},
		attachUUIDHeaderGRPC(),
	).Endpoint()

	requestConfigEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"RequestConfig",
		encodeProtobufAgentAPIRequest,
		decodeProtobufConfigResponse,
		kolide_agent.ConfigResponse{},
		attachUUIDHeaderGRPC(),
	).Endpoint()

	publishLogsEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"PublishLogs",
		encodeProtobufLogCollection,
		decodeProtobufAgentAPIResponse,
		kolide_agent.AgentApiResponse{},
		attachUUIDHeaderGRPC(),
	).Endpoint()

	requestQueriesEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"RequestQueries",
		encodeProtobufAgentAPIRequest,
		decodeProtobufQueryCollection,
		kolide_agent.QueryCollection{},
		attachUUIDHeaderGRPC(),
	).Endpoint()

	publishResultsEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"PublishResults",
		encodeProtobufResultCollection,
		decodeProtobufAgentAPIResponse,
		kolide_agent.AgentApiResponse{},
		attachUUIDHeaderGRPC(),
	).Endpoint()

	checkHealthEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"CheckHealth",
		encodeProtobufAgentAPIRequest,
		decodeProtobufHealthCheckResponse,
		kolide_agent.HealthCheckResponse{},
		attachUUIDHeaderGRPC(),
	).Endpoint()

	var client KolideService = KolideClient{
		RequestEnrollmentEndpoint: requestEnrollmentEndpoint,
		RequestConfigEndpoint:     requestConfigEndpoint,
		PublishLogsEndpoint:       publishLogsEndpoint,
		RequestQueriesEndpoint:    requestQueriesEndpoint,
		PublishResultsEndpoint:    publishResultsEndpoint,
		CheckHealthEndpoint:       checkHealthEndpoint,
	}

	client = LoggingMiddleware(logger)(client)
	// Wrap with UUID middleware after logger so that UUID is available in
	// the logger context.
	client = uuidMiddleware(client)

	return client
}
