package service

import (
	stdctx "context"

	"github.com/go-kit/kit/log"
	grpctransport "github.com/go-kit/kit/transport/grpc"
	pb "github.com/kolide/agent-api"
	"github.com/kolide/launcher/service/uuid"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc/metadata"
)

func parseUUID() grpctransport.ServerOption {
	return grpctransport.ServerBefore(
		func(ctx stdctx.Context, md metadata.MD) stdctx.Context {
			if hdr, ok := md["uuid"]; ok {
				ctx = uuid.NewContext(ctx, hdr[len(hdr)-1])
			}
			return ctx
		},
	)
}

func NewGRPCServer(endpoints KolideClient, logger log.Logger) pb.ApiServer {
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

func (s *grpcServer) RequestEnrollment(ctx context.Context, req *pb.EnrollmentRequest) (*pb.EnrollmentResponse, error) {
	_, rep, err := s.enrollment.ServeGRPC(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "request enrollment")
	}
	return rep.(*pb.EnrollmentResponse), nil
}

func (s *grpcServer) RequestConfig(ctx context.Context, req *pb.AgentApiRequest) (*pb.ConfigResponse, error) {
	_, rep, err := s.config.ServeGRPC(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "request config")
	}
	return rep.(*pb.ConfigResponse), nil
}

func (s *grpcServer) RequestQueries(ctx context.Context, req *pb.AgentApiRequest) (*pb.QueryCollection, error) {
	_, rep, err := s.queries.ServeGRPC(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "request queries")
	}
	return rep.(*pb.QueryCollection), nil
}

func (s *grpcServer) PublishLogs(ctx context.Context, req *pb.LogCollection) (*pb.AgentApiResponse, error) {
	_, rep, err := s.logs.ServeGRPC(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "publish logs")
	}
	return rep.(*pb.AgentApiResponse), nil
}

func (s *grpcServer) PublishResults(ctx context.Context, req *pb.ResultCollection) (*pb.AgentApiResponse, error) {
	_, rep, err := s.results.ServeGRPC(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "publish results")
	}
	return rep.(*pb.AgentApiResponse), nil
}

func (s *grpcServer) CheckHealth(ctx context.Context, req *pb.AgentApiRequest) (*pb.HealthCheckResponse, error) {
	_, rep, err := s.health.ServeGRPC(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "check health")
	}
	return rep.(*pb.HealthCheckResponse), nil
}
