package service

import (
	"context"

	"github.com/go-kit/kit/endpoint"
	pb "github.com/kolide/launcher/service/internal/launcherproto"
	"github.com/pkg/errors"
)

type configRequest struct {
	NodeKey string
}

type configResponse struct {
	ConfigJSONBlob string
	NodeInvalid    bool
	Err            error
}

func decodeGRPCConfigRequest(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*pb.AgentApiRequest)
	return configRequest{
		NodeKey: req.NodeKey,
	}, nil
}

func encodeGRPCConfigRequest(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(configRequest)
	return &pb.AgentApiRequest{
		NodeKey: req.NodeKey,
	}, nil
}

func decodeGRPCConfigResponse(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*pb.ConfigResponse)
	return configResponse{
		ConfigJSONBlob: req.ConfigJsonBlob,
		NodeInvalid:    req.NodeInvalid,
	}, nil
}

func encodeGRPCConfigResponse(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(configResponse)
	return &pb.ConfigResponse{
		ConfigJsonBlob: req.ConfigJSONBlob,
		NodeInvalid:    req.NodeInvalid,
	}, nil
}

func MakeRequestConfigEndpoint(svc KolideService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(configRequest)
		config, valid, err := svc.RequestConfig(ctx, req.NodeKey)
		return configResponse{
			ConfigJSONBlob: config,
			NodeInvalid:    valid,
			Err:            err,
		}, nil
	}
}

// RequestConfig implements KolideService.RequestConfig.
func (e Endpoints) RequestConfig(ctx context.Context, nodeKey string) (string, bool, error) {
	newCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request := configRequest{NodeKey: nodeKey}
	response, err := e.RequestConfigEndpoint(newCtx, request)
	if err != nil {
		return "", false, err
	}
	resp := response.(configResponse)
	return resp.ConfigJSONBlob, resp.NodeInvalid, resp.Err
}

func (s *grpcServer) RequestConfig(ctx context.Context, req *pb.AgentApiRequest) (*pb.ConfigResponse, error) {
	_, rep, err := s.config.ServeGRPC(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "request config")
	}
	return rep.(*pb.ConfigResponse), nil
}
