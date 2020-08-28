package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log/level"
	"github.com/go-kit/kit/transport/http/jsonrpc"
	"github.com/kolide/kit/contexts/uuid"
	"github.com/pkg/errors"

	pb "github.com/kolide/launcher/pkg/pb/launcher"
)

type configRequest struct {
	NodeKey string `json:"node_key"`
}

type configResponse struct {
	ConfigJSONBlob string `json:"config"`
	NodeInvalid    bool   `json:"node_invalid"`
	Err            error  `json:"error_code,omitempty"`
}

func decodeGRPCConfigRequest(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*pb.AgentApiRequest)
	return configRequest{
		NodeKey: req.NodeKey,
	}, nil
}

func decodeJSONRPCConfigRequest(_ context.Context, msg json.RawMessage) (interface{}, error) {
	var req configRequest

	if err := json.Unmarshal(msg, &req); err != nil {
		return nil, &jsonrpc.Error{
			Code:    -32000,
			Message: fmt.Sprintf("couldn't unmarshal body to configRequest: %s", err),
		}
	}
	return req, nil
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
	resp := &pb.ConfigResponse{
		ConfigJsonBlob: req.ConfigJSONBlob,
		NodeInvalid:    req.NodeInvalid,
	}
	return encodeResponse(resp, req.Err)
}

func encodeJSONRPCConfigResponse(_ context.Context, obj interface{}) (json.RawMessage, error) {
	res, ok := obj.(configResponse)
	if !ok {
		return encodeJSONResponse(nil, errors.Errorf("Asserting result to *configResponse failed. Got %T, %+v", obj, obj))
	}

	b, err := json.Marshal(res)
	return encodeJSONResponse(b, errors.Wrap(err, "marshal json response"))
}

func decodeJSONRPCConfigResponse(_ context.Context, res jsonrpc.Response) (interface{}, error) {
	if res.Error != nil {
		return nil, *res.Error // I'm undecided if we should errors.Wrap this or not.
	}

	var result configResponse
	err := json.Unmarshal(res.Result, &result)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshalling RequestConfig response")
	}
	return result, nil
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
		return nil, err
	}
	return rep.(*pb.ConfigResponse), nil
}

func (mw logmw) RequestConfig(ctx context.Context, nodeKey string) (config string, reauth bool, err error) {
	defer func(begin time.Time) {
		uuid, _ := uuid.FromContext(ctx)
		logger := level.Debug(mw.logger)
		if err != nil {
			logger = level.Info(mw.logger)
		}
		logger.Log(
			"method", "RequestConfig",
			"uuid", uuid,
			"config_size", len(config),
			"reauth", reauth,
			"err", err,
			"took", time.Since(begin),
		)
	}(time.Now())

	config, reauth, err = mw.next.RequestConfig(ctx, nodeKey)
	return config, reauth, err
}

func (mw uuidmw) RequestConfig(ctx context.Context, nodeKey string) (errcode string, reauth bool, err error) {
	ctx = uuid.NewContext(ctx, uuid.NewForRequest())
	return mw.next.RequestConfig(ctx, nodeKey)
}
