package service

import (
	"context"
	"net/http"

	"github.com/go-kit/kit/log"
	twirptransport "github.com/go-kit/kit/transport/twirp"
	"github.com/pkg/errors"

	pb "github.com/kolide/launcher/service/internal/launcherproto"
	"github.com/kolide/launcher/service/uuid"
)

const TwirpHTTPApiPathPrefix = pb.ApiPathPrefix

func parseUUIDHeaderTwirp() twirptransport.ServerOption {
	return twirptransport.ServerBefore(
		func(ctx context.Context, header http.Header) context.Context {
			return uuid.NewContext(ctx, header.Get("uuid"))
		},
	)
}

func NewTwirpHTTPHandler(endpoints KolideClient, logger log.Logger) http.Handler {
	options := []twirptransport.ServerOption{
		twirptransport.ServerErrorLogger(logger),
		parseUUIDHeaderTwirp(),
	}
	twirpServer := &twirpServer{
		enrollment: twirptransport.NewServer(
			endpoints.RequestEnrollmentEndpoint,
			decodeProtobufEnrollmentRequest,
			encodeProtobufEnrollmentResponse,
			options...,
		),
		config: twirptransport.NewServer(
			endpoints.RequestConfigEndpoint,
			decodeProtobufAgentAPIRequest,
			encodeProtobufConfigResponse,
			options...,
		),
		queries: twirptransport.NewServer(
			endpoints.RequestQueriesEndpoint,
			decodeProtobufAgentAPIRequest,
			encodeProtobufQueryCollection,
			options...,
		),
		logs: twirptransport.NewServer(
			endpoints.PublishLogsEndpoint,
			decodeProtobufLogCollection,
			encodeProtobufAgentAPIResponse,
			options...,
		),
		results: twirptransport.NewServer(
			endpoints.PublishResultsEndpoint,
			decodeProtobufResultCollection,
			encodeProtobufAgentAPIResponse,
			options...,
		),
		health: twirptransport.NewServer(
			endpoints.CheckHealthEndpoint,
			decodeProtobufAgentAPIRequest,
			encodeProtobufHealthcheckResponse,
			options...,
		),
	}
	return pb.NewApiServer(twirpServer, nil)
}

type twirpServer struct {
	enrollment twirptransport.Handler
	config     twirptransport.Handler
	queries    twirptransport.Handler
	logs       twirptransport.Handler
	results    twirptransport.Handler
	health     twirptransport.Handler
}

func (s *twirpServer) RequestEnrollment(ctx context.Context, req *pb.EnrollmentRequest) (*pb.EnrollmentResponse, error) {
	_, rep, err := s.enrollment.ServeTwirp(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "request enrollment")
	}
	return rep.(*pb.EnrollmentResponse), nil
}

func (s *twirpServer) RequestConfig(ctx context.Context, req *pb.AgentApiRequest) (*pb.ConfigResponse, error) {
	_, rep, err := s.config.ServeTwirp(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "request config")
	}
	return rep.(*pb.ConfigResponse), nil
}

func (s *twirpServer) RequestQueries(ctx context.Context, req *pb.AgentApiRequest) (*pb.QueryCollection, error) {
	_, rep, err := s.queries.ServeTwirp(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "request queries")
	}
	return rep.(*pb.QueryCollection), nil
}

func (s *twirpServer) PublishLogs(ctx context.Context, req *pb.LogCollection) (*pb.AgentApiResponse, error) {
	_, rep, err := s.logs.ServeTwirp(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "publish logs")
	}
	return rep.(*pb.AgentApiResponse), nil
}

func (s *twirpServer) PublishResults(ctx context.Context, req *pb.ResultCollection) (*pb.AgentApiResponse, error) {
	_, rep, err := s.results.ServeTwirp(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "publish results")
	}
	return rep.(*pb.AgentApiResponse), nil
}

func (s *twirpServer) CheckHealth(ctx context.Context, req *pb.AgentApiRequest) (*pb.HealthCheckResponse, error) {
	_, rep, err := s.health.ServeTwirp(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "check health")
	}
	return rep.(*pb.HealthCheckResponse), nil
}
