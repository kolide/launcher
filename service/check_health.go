package service

import (
	"context"
	"time"

	"github.com/go-kit/kit/endpoint"
	"github.com/kolide/kit/contexts/uuid"

	"github.com/pkg/errors"

	pb "github.com/kolide/launcher/service/internal/launcherproto"
)

type healthcheckRequest struct{}
type healthcheckResponse struct {
	Status int32
	Err    error
}

func decodeGRPCHealthCheckRequest(_ context.Context, grpcReq interface{}) (interface{}, error) {
	return healthcheckRequest{}, nil
}

func encodeGRPCHealcheckRequest(_ context.Context, request interface{}) (interface{}, error) {
	return &pb.AgentApiRequest{}, nil
}

func decodeGRPCHealthCheckResponse(_ context.Context, grpcReq interface{}) (interface{}, error) {
	resp := grpcReq.(*pb.HealthCheckResponse)
	return healthcheckResponse{
		Status: int32(resp.GetStatus()),
	}, nil
}

func encodeGRPCHealthcheckResponse(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(healthcheckResponse)
	return &pb.HealthCheckResponse{
		Status: pb.HealthCheckResponse_ServingStatus(req.Status),
	}, nil
}

func MakeCheckHealthEndpoint(svc KolideService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		status, err := svc.CheckHealth(ctx)
		return healthcheckResponse{
			Status: status,
			Err:    err,
		}, nil
	}
}

func (e Endpoints) CheckHealth(ctx context.Context) (int32, error) {
	newCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request := healthcheckRequest{}
	response, err := e.CheckHealthEndpoint(newCtx, request)
	if err != nil {
		return 0, err
	}
	resp := response.(healthcheckResponse)
	return resp.Status, resp.Err
}

func (s *grpcServer) CheckHealth(ctx context.Context, req *pb.AgentApiRequest) (*pb.HealthCheckResponse, error) {
	_, rep, err := s.health.ServeGRPC(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "check health")
	}
	return rep.(*pb.HealthCheckResponse), nil
}

func (mw logmw) CheckHealth(ctx context.Context) (status int32, err error) {
	defer func(begin time.Time) {
		uuid, _ := uuid.FromContext(ctx)
		mw.logger.Log(
			"method", "CheckHealth",
			"uuid", uuid,
			"status", status,
			"err", err,
			"took", time.Since(begin),
		)
	}(time.Now())
	status, err = mw.next.CheckHealth(ctx)
	return
}

func (mw uuidmw) CheckHealth(ctx context.Context) (status int32, err error) {
	ctx = uuid.NewContext(ctx, uuid.NewForRequest())
	return mw.next.CheckHealth(ctx)
}
