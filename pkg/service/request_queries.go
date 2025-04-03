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
	"github.com/osquery/osquery-go/plugin/distributed"
)

type queriesRequest struct {
	NodeKey string `json:"node_key"`
}

type queryCollectionResponse struct {
	jsonRpcResponse
	Queries     distributed.GetQueriesResult
	NodeInvalid bool   `json:"node_invalid"`
	ErrorCode   string `json:"error_code,omitempty"`
	Err         error  `json:"err,omitempty"`
}

func decodeJSONRPCQueryCollection(_ context.Context, res jsonrpc.Response) (interface{}, error) {
	if res.Error != nil {
		return nil, *res.Error
	}
	var result queryCollectionResponse
	err := json.Unmarshal(res.Result, &result)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling RequestQueries response: %w", err)
	}

	return result, nil
}

func decodeGRPCQueriesRequest(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*pb.AgentApiRequest)
	return queriesRequest{
		NodeKey: req.NodeKey,
	}, nil
}

func decodeJSONRPCQueriesRequest(_ context.Context, msg json.RawMessage) (interface{}, error) {
	var req queriesRequest

	if err := json.Unmarshal(msg, &req); err != nil {
		return nil, &jsonrpc.Error{
			Code:    -32000,
			Message: fmt.Sprintf("couldn't unmarshal body to queriesRequest: %s", err),
		}
	}
	return req, nil
}

func encodeGRPCQueriesRequest(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(queriesRequest)
	return &pb.AgentApiRequest{
		NodeKey: req.NodeKey,
	}, nil
}

func decodeGRPCQueryCollection(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*pb.QueryCollection)
	queries := distributed.GetQueriesResult{
		Queries:   map[string]string{},
		Discovery: map[string]string{},
	}
	for _, query := range req.Queries {
		queries.Queries[query.Id] = query.Query
	}
	return queryCollectionResponse{
		jsonRpcResponse: jsonRpcResponse{
			DisableDevice: req.DisableDevice,
		},
		Queries:     queries,
		NodeInvalid: req.NodeInvalid,
	}, nil
}

func encodeGRPCQueryCollection(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(queryCollectionResponse)
	queries := make([]*pb.QueryCollection_Query, 0, len(req.Queries.Queries))
	for id, query := range req.Queries.Queries {
		queries = append(queries,
			&pb.QueryCollection_Query{
				Id:    id,
				Query: query,
			},
		)
	}
	resp := &pb.QueryCollection{
		Queries:       queries,
		NodeInvalid:   req.NodeInvalid,
		DisableDevice: req.DisableDevice,
	}
	return encodeResponse(resp, req.Err)
}

func encodeJSONRPCQueryCollection(_ context.Context, obj interface{}) (json.RawMessage, error) {
	res, ok := obj.(queryCollectionResponse)
	if !ok {
		return encodeJSONResponse(nil, fmt.Errorf("asserting result to *queryCollectionResponse failed. Got %T, %+v", obj, obj))
	}

	b, err := json.Marshal(res)
	if err != nil {
		return encodeJSONResponse(b, fmt.Errorf("marshal json response: %w", err))
	}

	return encodeJSONResponse(b, nil)
}

func MakeRequestQueriesEndpoint(svc KolideService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(queriesRequest)
		result, valid, err := svc.RequestQueries(ctx, req.NodeKey)
		if err != nil {
			return queryCollectionResponse{Err: err}, nil
		}
		resp := queryCollectionResponse{
			NodeInvalid: valid,
			Err:         err,
		}
		if result != nil {
			resp.Queries = *result
		}
		return resp, nil
	}
}

// RequestQueries implements KolideService.RequestQueries
func (e Endpoints) RequestQueries(ctx context.Context, nodeKey string) (*distributed.GetQueriesResult, bool, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	newCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request := queriesRequest{NodeKey: nodeKey}
	response, err := e.RequestQueriesEndpoint(newCtx, request)
	if err != nil {
		return nil, false, err
	}
	resp := response.(queryCollectionResponse)

	if resp.DisableDevice {
		return nil, false, ErrDeviceDisabled{}
	}

	return &resp.Queries, resp.NodeInvalid, resp.Err
}

func (s *grpcServer) RequestQueries(ctx context.Context, req *pb.AgentApiRequest) (*pb.QueryCollection, error) {
	_, rep, err := s.queries.ServeGRPC(ctx, req)
	if err != nil {
		return nil, err
	}
	return rep.(*pb.QueryCollection), nil
}

func (mw logmw) RequestQueries(ctx context.Context, nodeKey string) (res *distributed.GetQueriesResult, reauth bool, err error) {
	defer func(begin time.Time) {
		resJSON, _ := json.Marshal(res)
		uuid, _ := uuid.FromContext(ctx)

		message := "success"
		if err != nil {
			message = "failure"
		}

		mw.knapsack.Slogger().Log(ctx, levelForError(err), // nolint:sloglint // it's fine to not have a constant or literal here
			message,
			"method", "RequestQueries",
			"uuid", uuid,
			"res", string(resJSON),
			"reauth", reauth,
			"err", err,
			"took", time.Since(begin),
		)
	}(time.Now())

	res, reauth, err = mw.next.RequestQueries(ctx, nodeKey)
	return res, reauth, err
}

func (mw uuidmw) RequestQueries(ctx context.Context, nodeKey string) (res *distributed.GetQueriesResult, reauth bool, err error) {
	ctx = uuid.NewContext(ctx, uuid.NewForRequest())
	return mw.next.RequestQueries(ctx, nodeKey)
}
