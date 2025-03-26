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
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/distributed"
)

type resultCollection struct {
	NodeKey string `json:"node_key"`
	Results []distributed.Result
}

type publishResultsResponse struct {
	jsonRpcResponse
	Message     string `json:"message"`
	NodeInvalid bool   `json:"node_invalid"`
	ErrorCode   string `json:"error_code,omitempty"`
	Err         error  `json:"err,omitempty"`
}

func decodeGRPCResultCollection(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*pb.ResultCollection)

	results := make([]distributed.Result, 0, len(req.Results))
	for _, result := range req.Results {
		// Iterate results
		rows := make([]map[string]string, 0, len(result.Rows))
		for _, row := range result.Rows {
			// Extract rows into map[string]string
			rowMap := make(map[string]string, len(row.Columns))
			for _, col := range row.Columns {
				rowMap[col.Name] = col.Value
			}
			rows = append(rows, rowMap)
		}
		results = append(results,
			distributed.Result{
				QueryName: result.Id,
				Status:    int(result.Status),
				Rows:      rows,
			},
		)
	}

	return resultCollection{
		Results: results,
		NodeKey: req.NodeKey,
	}, nil
}

func decodeJSONRPCResultCollection(_ context.Context, msg json.RawMessage) (interface{}, error) {
	var req resultCollection

	if err := json.Unmarshal(msg, &req); err != nil {
		return nil, &jsonrpc.Error{
			Code:    -32000,
			Message: fmt.Sprintf("couldn't unmarshal body to resultCollection: %s", err),
		}
	}
	return req, nil
}

func decodeJSONRPCPublishResultsResponse(_ context.Context, res jsonrpc.Response) (interface{}, error) {
	if res.Error != nil {
		return nil, *res.Error
	}
	var result publishResultsResponse
	err := json.Unmarshal(res.Result, &result)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling PublishResults response: %w", err)
	}

	return result, nil
}

func encodeGRPCResultCollection(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(resultCollection)

	results := make([]*pb.ResultCollection_Result, 0, len(req.Results))
	for _, result := range req.Results {
		// Iterate results
		rows := make([]*pb.ResultCollection_Result_ResultRow, 0, len(result.Rows))
		for _, row := range result.Rows {
			// Extract rows into columns
			rowCols := make([]*pb.ResultCollection_Result_ResultRow_Column, 0, len(row))
			for col, val := range row {
				rowCols = append(rowCols, &pb.ResultCollection_Result_ResultRow_Column{
					Name:  col,
					Value: val,
				})
			}
			rows = append(rows, &pb.ResultCollection_Result_ResultRow{Columns: rowCols})
		}
		results = append(results,
			&pb.ResultCollection_Result{
				Id:     result.QueryName,
				Status: int32(result.Status),
				Rows:   rows,
			},
		)
	}

	return &pb.ResultCollection{
		NodeKey: req.NodeKey,
		Results: results,
	}, nil
}

func decodeGRPCPublishResultsResponse(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*pb.AgentApiResponse)
	return publishResultsResponse{
		jsonRpcResponse: jsonRpcResponse{
			DisableDevice: req.DisableDevice,
		},
		Message:     req.Message,
		ErrorCode:   req.ErrorCode,
		NodeInvalid: req.NodeInvalid,
	}, nil
}

func encodeGRPCPublishResultsResponse(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(publishResultsResponse)
	resp := &pb.AgentApiResponse{
		Message:       req.Message,
		ErrorCode:     req.ErrorCode,
		NodeInvalid:   req.NodeInvalid,
		DisableDevice: req.DisableDevice,
	}
	return encodeResponse(resp, req.Err)
}

func encodeJSONRPCPublishResultsResponse(_ context.Context, obj interface{}) (json.RawMessage, error) {
	res, ok := obj.(publishResultsResponse)
	if !ok {
		return encodeJSONResponse(nil, fmt.Errorf("asserting result to *publishResultsResponse failed. Got %T, %+v", obj, obj))
	}

	b, err := json.Marshal(res)
	return encodeJSONResponse(b, fmt.Errorf("marshal json response: %w", err))
}

func MakePublishResultsEndpoint(svc KolideService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(resultCollection)
		message, errcode, valid, err := svc.PublishResults(ctx, req.NodeKey, req.Results)
		return publishResultsResponse{
			Message:     message,
			ErrorCode:   errcode,
			NodeInvalid: valid,
			Err:         err,
		}, nil
	}
}

// PublishResults implements KolideService.PublishResults
func (e Endpoints) PublishResults(ctx context.Context, nodeKey string, results []distributed.Result) (string, string, bool, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	newCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	request := resultCollection{NodeKey: nodeKey, Results: results}
	response, err := e.PublishResultsEndpoint(newCtx, request)
	if err != nil {
		return "", "", false, err
	}
	resp := response.(publishResultsResponse)

	if resp.DisableDevice {
		return "", "", false, ErrDeviceDisabled{}
	}

	return resp.Message, resp.ErrorCode, resp.NodeInvalid, resp.Err
}

func (s *grpcServer) PublishResults(ctx context.Context, req *pb.ResultCollection) (*pb.AgentApiResponse, error) {
	_, rep, err := s.results.ServeGRPC(ctx, req)
	if err != nil {
		return nil, err
	}
	return rep.(*pb.AgentApiResponse), nil
}

func (mw logmw) PublishResults(ctx context.Context, nodeKey string, results []distributed.Result) (message, errcode string, reauth bool, err error) {
	defer func(begin time.Time) {
		resJSON, _ := json.Marshal(results)

		uuid, _ := uuid.FromContext(ctx)

		if message == "" {
			if err == nil {
				message = "success"
			} else {
				message = "failure"
			}
		}

		mw.knapsack.Slogger().Log(ctx, levelForError(err), message, // nolint:sloglint // it's fine to not have a constant or literal here
			"method", "PublishResults",
			"uuid", uuid,
			"results_truncated", trivialTruncate(string(resJSON), 200),
			"result_count", len(results),
			"result_size", len(resJSON),
			"errcode", errcode,
			"reauth", reauth,
			"err", err,
			"took", time.Since(begin),
		)

		for _, r := range results {
			if r.QueryStats == nil {
				continue
			}

			mw.knapsack.Slogger().Log(ctx, slog.LevelInfo,
				"received distributed query stats",
				"query_name", r.QueryName,
				"query_status", r.Status,
				"wall_time_ms", r.QueryStats.WallTimeMs,
				"user_time", r.QueryStats.UserTime,
				"system_time", r.QueryStats.SystemTime,
				"memory", r.QueryStats.Memory,
				"long_running", r.QueryStats.WallTimeMs > 5000,
			)
		}
	}(time.Now())

	message, errcode, reauth, err = mw.next.PublishResults(ctx, nodeKey, results)
	return message, errcode, reauth, err
}

func (mw uuidmw) PublishResults(ctx context.Context, nodeKey string, results []distributed.Result) (message, errcode string, reauth bool, err error) {
	ctx = uuid.NewContext(ctx, uuid.NewForRequest())
	return mw.next.PublishResults(ctx, nodeKey, results)
}

// trivialTruncate performs a trivial truncate operation on strings. Because it's string based, it may not handle
// multibyte characters correctly. Note that this actually returns a string length of maxLen +3, but that's okay
// because it's only used to keep logs from being too huge.
func trivialTruncate(str string, maxLen int) string {
	if len(str) <= maxLen {
		return str
	}

	return str[:maxLen] + "..."

}
