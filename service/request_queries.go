package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-kit/kit/endpoint"
	"github.com/kolide/kit/contexts/uuid"
	"github.com/kolide/osquery-go/plugin/distributed"
	"github.com/pkg/errors"

	pb "github.com/kolide/launcher/service/internal/launcherproto"
)

type queriesRequest struct {
	NodeKey string
}

type queryCollectionResponse struct {
	Queries     distributed.GetQueriesResult
	NodeInvalid bool
	Err         error
}

func decodeGRPCQueriesRequest(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*pb.AgentApiRequest)
	return queriesRequest{
		NodeKey: req.NodeKey,
	}, nil
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
	return &pb.QueryCollection{
		Queries:     queries,
		NodeInvalid: req.NodeInvalid,
	}, nil
}

func MakeRequestQueriesEndpoint(svc KolideService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(queriesRequest)
		result, valid, err := svc.RequestQueries(ctx, req.NodeKey)
		if err != nil {
			return queryCollectionResponse{Err: err}, nil
		}
		return queryCollectionResponse{
			Queries:     *result,
			NodeInvalid: valid,
			Err:         err,
		}, nil
	}
}

// RequestQueries implements KolideService.RequestQueries
func (e Endpoints) RequestQueries(ctx context.Context, nodeKey string) (*distributed.GetQueriesResult, bool, error) {
	newCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request := queriesRequest{NodeKey: nodeKey}
	response, err := e.RequestQueriesEndpoint(newCtx, request)
	if err != nil {
		return nil, false, err
	}
	resp := response.(queryCollectionResponse)
	return &resp.Queries, resp.NodeInvalid, resp.Err
}

func (s *grpcServer) RequestQueries(ctx context.Context, req *pb.AgentApiRequest) (*pb.QueryCollection, error) {
	_, rep, err := s.queries.ServeGRPC(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "request queries")
	}
	return rep.(*pb.QueryCollection), nil
}

func (mw logmw) RequestQueries(ctx context.Context, nodeKey string) (res *distributed.GetQueriesResult, reauth bool, err error) {
	defer func(begin time.Time) {
		resJSON, _ := json.Marshal(res)
		uuid, _ := uuid.FromContext(ctx)
		mw.logger.Log(
			"method", "RequestQueries",
			"uuid", uuid,
			"res", string(resJSON),
			"reauth", reauth,
			"err", err,
			"took", time.Since(begin),
		)
	}(time.Now())

	res, reauth, err = mw.next.RequestQueries(ctx, nodeKey)
	return
}

func (mw uuidmw) RequestQueries(ctx context.Context, nodeKey string) (res *distributed.GetQueriesResult, reauth bool, err error) {
	ctx = uuid.NewContext(ctx, uuid.NewForRequest())
	return mw.next.RequestQueries(ctx, nodeKey)
}
