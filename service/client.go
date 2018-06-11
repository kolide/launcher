package service

import (
	"github.com/go-kit/kit/log"
	grpctransport "github.com/go-kit/kit/transport/grpc"
	"github.com/kolide/kit/contexts/uuid"
	"google.golang.org/grpc"

	pb "github.com/kolide/launcher/service/internal/launcherproto"
)

// New creates a new Kolide Client (implementation of the KolideService
// interface) using the provided gRPC client connection.
func New(conn *grpc.ClientConn, logger log.Logger) KolideService {
	requestEnrollmentEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"RequestEnrollment",
		encodeGRPCEnrollmentRequest,
		decodeGRPCEnrollmentResponse,
		pb.EnrollmentResponse{},
		uuid.Attach(),
	).Endpoint()

	requestConfigEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"RequestConfig",
		encodeGRPCConfigRequest,
		decodeGRPCConfigResponse,
		pb.ConfigResponse{},
		uuid.Attach(),
	).Endpoint()

	publishLogsEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"PublishLogs",
		encodeGRPCLogCollection,
		decodeGRPCPublishLogsResponse,
		pb.AgentApiResponse{},
		uuid.Attach(),
	).Endpoint()

	requestQueriesEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"RequestQueries",
		encodeGRPCQueriesRequest,
		decodeGRPCQueryCollection,
		pb.QueryCollection{},
		uuid.Attach(),
	).Endpoint()

	publishResultsEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"PublishResults",
		encodeGRPCResultCollection,
		decodeGRPCPublishResultsResponse,
		pb.AgentApiResponse{},
		uuid.Attach(),
	).Endpoint()

	checkHealthEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"CheckHealth",
		encodeGRPCHealcheckRequest,
		decodeGRPCHealthCheckResponse,
		pb.HealthCheckResponse{},
		uuid.Attach(),
	).Endpoint()

	var client KolideService = Endpoints{
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
