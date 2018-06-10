package service

import (
	"context"

	"github.com/go-kit/kit/log"
	grpctransport "github.com/go-kit/kit/transport/grpc"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	pb "github.com/kolide/launcher/service/internal/launcherproto"
	"github.com/kolide/launcher/service/uuid"
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

func NewGRPCServer(endpoints Endpoints, logger log.Logger) pb.ApiServer {
	options := []grpctransport.ServerOption{
		grpctransport.ServerErrorLogger(logger),
		parseUUID(),
	}
	return &grpcServer{
		enrollment: grpctransport.NewServer(
			endpoints.RequestEnrollmentEndpoint,
			decodeGRPCEnrollmentRequest,
			encodeGRPCEnrollmentResponse,
			options...,
		),
		config: grpctransport.NewServer(
			endpoints.RequestConfigEndpoint,
			decodeGRPCAgentAPIRequest,
			encodeGRPCConfigResponse,
			options...,
		),
		queries: grpctransport.NewServer(
			endpoints.RequestQueriesEndpoint,
			decodeGRPCAgentAPIRequest,
			encodeGRPCQueryCollection,
			options...,
		),
		logs: grpctransport.NewServer(
			endpoints.PublishLogsEndpoint,
			decodeGRPCLogCollection,
			encodeGRPCAgentAPIResponse,
			options...,
		),
		results: grpctransport.NewServer(
			endpoints.PublishResultsEndpoint,
			decodeGRPCResultCollection,
			encodeGRPCAgentAPIResponse,
			options...,
		),
		health: grpctransport.NewServer(
			endpoints.CheckHealthEndpoint,
			decodeGRPCAgentAPIRequest,
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

func (s *grpcServer) CheckHealth(ctx context.Context, req *pb.AgentApiRequest) (*pb.HealthCheckResponse, error) {
	_, rep, err := s.health.ServeGRPC(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "check health")
	}
	return rep.(*pb.HealthCheckResponse), nil
}

func RegisterGRPCServer(grpcServer *grpc.Server, apiServer pb.ApiServer) {
	pb.RegisterApiServer(grpcServer, apiServer)
}
