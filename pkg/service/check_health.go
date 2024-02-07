package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/transport/http/jsonrpc"
	"github.com/kolide/kit/contexts/uuid"

	pb "github.com/kolide/launcher/pkg/pb/launcher"
)

type healthcheckRequest struct{}
type healthcheckResponse struct {
	Status    int32  `json:"status"`
	ErrorCode string `json:"error_code,omitempty"`
	Err       error  `json:"err,omitempty"`
}

func decodeGRPCHealthCheckRequest(_ context.Context, grpcReq interface{}) (interface{}, error) {
	return healthcheckRequest{}, nil
}

func decodeJSONRPCHealthCheckRequest(_ context.Context, msg json.RawMessage) (interface{}, error) {
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

func encodeJSONRPCHealthcheckResponse(_ context.Context, obj interface{}) (json.RawMessage, error) {
	res, ok := obj.(healthcheckResponse)
	if !ok {
		return encodeJSONResponse(nil, fmt.Errorf("asserting result to *healthcheckResponse failed. Got %T, %+v", obj, obj))
	}

	b, err := json.Marshal(res)
	if err != nil {
		return encodeJSONResponse(b, fmt.Errorf("marshal json response: %w", err))
	}

	return encodeJSONResponse(b, nil)
}

func decodeJSONRPCHealthCheckResponse(_ context.Context, res jsonrpc.Response) (interface{}, error) {
	if res.Error != nil {
		return nil, *res.Error
	}
	var result healthcheckResponse
	err := json.Unmarshal(res.Result, &result)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling CheckHealth response: %w", err)
	}

	return result, nil
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
		return nil, fmt.Errorf("check health: %w", err)
	}
	return rep.(*pb.HealthCheckResponse), nil
}

func (mw logmw) CheckHealth(ctx context.Context) (status int32, err error) {
	defer func(begin time.Time) {
		uuid, _ := uuid.FromContext(ctx)
		mw.knapsack.Slogger().Log(ctx, slog.LevelDebug, "check health",
			"method", "CheckHealth",
			"uuid", uuid,
			"status", status,
			"err", err,
			"took", time.Since(begin),
		)
	}(time.Now())
	status, err = mw.next.CheckHealth(ctx)
	return status, err
}

func (mw uuidmw) CheckHealth(ctx context.Context) (status int32, err error) {
	ctx = uuid.NewContext(ctx, uuid.NewForRequest())
	return mw.next.CheckHealth(ctx)
}
