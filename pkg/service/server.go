package service

import (
	"context"

	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
	grpctransport "github.com/go-kit/kit/transport/grpc"
	"github.com/kolide/kit/contexts/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	pb "github.com/kolide/launcher/pkg/pb/launcher"
)

func parseUUID() grpctransport.ServerOption {
	return grpctransport.ServerBefore(
		func(ctx context.Context, md metadata.MD) context.Context {
			if hdr, ok := md["uuid"]; ok {
				ctx = uuid.NewContext(ctx, hdr[len(hdr)-1])
			}
			return ctx
		},
	)
}

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

func NewGRPCServer(endpoints Endpoints, logger log.Logger, options ...grpctransport.ServerOption) pb.ApiServer {
	options = append(options, parseUUID())
	return &grpcServer{
		enrollment: grpctransport.NewServer(
			endpoints.RequestEnrollmentEndpoint,
			decodeGRPCEnrollmentRequest,
			encodeGRPCEnrollmentResponse,
			options...,
		),
		config: grpctransport.NewServer(
			endpoints.RequestConfigEndpoint,
			decodeGRPCConfigRequest,
			encodeGRPCConfigResponse,
			options...,
		),
		queries: grpctransport.NewServer(
			endpoints.RequestQueriesEndpoint,
			decodeGRPCQueriesRequest,
			encodeGRPCQueryCollection,
			options...,
		),
		logs: grpctransport.NewServer(
			endpoints.PublishLogsEndpoint,
			decodeGRPCLogCollection,
			encodeGRPCPublishLogsResponse,
			options...,
		),
		results: grpctransport.NewServer(
			endpoints.PublishResultsEndpoint,
			decodeGRPCResultCollection,
			encodeGRPCPublishResultsResponse,
			options...,
		),
		health: grpctransport.NewServer(
			endpoints.CheckHealthEndpoint,
			decodeGRPCHealthCheckRequest,
			encodeGRPCHealthcheckResponse,
			options...,
		),
	}
}

type grpcServer struct {
	enrollment grpctransport.Handler
	config     grpctransport.Handler
	queries    grpctransport.Handler
	logs       grpctransport.Handler
	results    grpctransport.Handler
	health     grpctransport.Handler
}

func RegisterGRPCServer(grpcServer *grpc.Server, apiServer pb.ApiServer) {
	pb.RegisterApiServer(grpcServer, apiServer)
}
