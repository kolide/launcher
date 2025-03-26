package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/transport/http/jsonrpc"
	"github.com/kolide/kit/contexts/uuid"

	pb "github.com/kolide/launcher/pkg/pb/launcher"
	"github.com/kolide/launcher/pkg/traces"
)

type configRequest struct {
	NodeKey string `json:"node_key"`
}

type configResponse struct {
	jsonRpcResponse
	ConfigJSONBlob string `json:"config"`
	NodeInvalid    bool   `json:"node_invalid"`
	ErrorCode      string `json:"error_code,omitempty"`
	Err            error  `json:"err,omitempty"`
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
		jsonRpcResponse: jsonRpcResponse{
			DisableDevice: req.DisableDevice,
		},
		ConfigJSONBlob: req.ConfigJsonBlob,
		NodeInvalid:    req.NodeInvalid,
	}, nil
}

func encodeGRPCConfigResponse(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(configResponse)
	resp := &pb.ConfigResponse{
		ConfigJsonBlob: req.ConfigJSONBlob,
		NodeInvalid:    req.NodeInvalid,
		DisableDevice:  req.DisableDevice,
	}
	return encodeResponse(resp, req.Err)
}

func encodeJSONRPCConfigResponse(_ context.Context, obj interface{}) (json.RawMessage, error) {
	res, ok := obj.(configResponse)
	if !ok {
		return encodeJSONResponse(nil, fmt.Errorf("asserting result to *configResponse failed. Got %T, %+v", obj, obj))
	}

	b, err := json.Marshal(res)
	if err != nil {
		return encodeJSONResponse(b, fmt.Errorf("marshal json response: %w", err))
	}

	return encodeJSONResponse(b, nil)
}

func decodeJSONRPCConfigResponse(_ context.Context, res jsonrpc.Response) (interface{}, error) {
	if res.Error != nil {
		return nil, *res.Error // I'm undecided if we should errors.Wrap this or not.
	}

	var result configResponse
	err := json.Unmarshal(res.Result, &result)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling RequestConfig response: %w", err)
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
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	newCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request := configRequest{NodeKey: nodeKey}
	response, err := e.RequestConfigEndpoint(newCtx, request)
	if err != nil {
		return "", false, err
	}
	resp := response.(configResponse)

	if resp.DisableDevice {
		return "", false, ErrDeviceDisabled{}
	}

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

		message := "success"
		if err != nil {
			message = "failure"
		}

		mw.knapsack.Slogger().Log(ctx, levelForError(err), message, // nolint:sloglint // it's fine to not have a constant or literal here
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
