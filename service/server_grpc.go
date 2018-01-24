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
		practices: grpctransport.NewServer(
			endpoints.RequestPracticesEndpoint,
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
	practices  grpctransport.Handler
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

func (s *grpcServer) RequestPractices(ctx context.Context, req *pb.AgentApiRequest) (*pb.QueryCollection, error) {
	_, rep, err := s.practices.ServeGRPC(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "request practices")
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

func RegisterGRPCServer(grpcServer *grpc.Server, apiServer pb.ApiServer) {
	pb.RegisterApiServer(grpcServer, apiServer)
}
