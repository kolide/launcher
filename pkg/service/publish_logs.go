package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/transport/http/jsonrpc"
	"github.com/kolide/kit/contexts/uuid"
	"github.com/osquery/osquery-go/plugin/logger"

	pb "github.com/kolide/launcher/pkg/pb/launcher"
)

type contextKey string

const (
	// PublicationCtxKey is used to set the relevant thresholds in context for reporting when logs are published
	PublicationCtxKey contextKey = "log_publication_state"
)

type logCollection struct {
	NodeKey string `json:"node_key"`
	LogType logger.LogType
	Logs    []string
}

type publishLogsResponse struct {
	Message     string `json:"message"`
	NodeInvalid bool   `json:"node_invalid"`
	ErrorCode   string `json:"error_code,omitempty"`
	Err         error  `json:"err,omitempty"`
}

func decodeGRPCLogCollection(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*pb.LogCollection)
	logs := make([]string, 0, len(req.Logs))
	for _, log := range req.Logs {
		logs = append(logs, log.Data)
	}

	// Note: The conversion here is lossy because the osquery-go logType has more
	// enum values than kolide_agent.
	// For now this should be enough because we don't use the Agent LogType anywhere.
	// A more robust fix should come from fixing https://github.com/kolide/launcher/issues/183
	var typ logger.LogType
	switch req.LogType {
	case pb.LogCollection_STATUS:
		typ = logger.LogTypeStatus
	case pb.LogCollection_RESULT:
		typ = logger.LogTypeSnapshot
	default:
		return logCollection{}, fmt.Errorf("unsupported log type %d", req.LogType)
	}

	return logCollection{
		NodeKey: req.NodeKey,
		LogType: typ,
		Logs:    logs,
	}, nil
}

func decodeJSONRPCLogCollection(_ context.Context, msg json.RawMessage) (interface{}, error) {
	var req logCollection

	if err := json.Unmarshal(msg, &req); err != nil {
		return nil, &jsonrpc.Error{
			Code:    -32000,
			Message: fmt.Sprintf("couldn't unmarshal body to logCollection: %s", err),
		}
	}
	return req, nil
}

func encodeGRPCLogCollection(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(logCollection)
	logs := make([]*pb.LogCollection_Log, 0, len(req.Logs))
	for _, log := range req.Logs {
		logs = append(logs, &pb.LogCollection_Log{Data: log})
	}

	var typ pb.LogCollection_LogType
	switch req.LogType {
	case logger.LogTypeStatus:
		typ = pb.LogCollection_STATUS
	case logger.LogTypeString, logger.LogTypeSnapshot:
		typ = pb.LogCollection_RESULT
	default:
		typ = pb.LogCollection_AGENT
	}

	return &pb.LogCollection{
		NodeKey: req.NodeKey,
		LogType: typ,
		Logs:    logs,
	}, nil
}

func decodeGRPCPublishLogsResponse(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*pb.AgentApiResponse)
	return publishLogsResponse{
		Message:     req.Message,
		ErrorCode:   req.ErrorCode,
		NodeInvalid: req.NodeInvalid,
	}, nil
}

func encodeGRPCPublishLogsResponse(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(publishLogsResponse)
	resp := &pb.AgentApiResponse{
		Message:     req.Message,
		ErrorCode:   req.ErrorCode,
		NodeInvalid: req.NodeInvalid,
	}
	return encodeResponse(resp, req.Err)
}

func encodeJSONRPCPublishLogsResponse(_ context.Context, obj interface{}) (json.RawMessage, error) {
	res, ok := obj.(publishLogsResponse)
	if !ok {
		return encodeJSONResponse(nil, fmt.Errorf("asserting result to *publishLogsResponse failed. Got %T, %+v", obj, obj))
	}

	b, err := json.Marshal(res)
	return encodeJSONResponse(b, fmt.Errorf("marshal json response: %w", err))
}

func decodeJSONRPCPublishLogsResponse(_ context.Context, res jsonrpc.Response) (interface{}, error) {
	if res.Error != nil {
		return nil, *res.Error
	}
	var result publishLogsResponse
	err := json.Unmarshal(res.Result, &result)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling PublishLogs response: %w", err)
	}

	return result, nil
}

func MakePublishLogsEndpoint(svc KolideService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(logCollection)
		message, errcode, valid, err := svc.PublishLogs(ctx, req.NodeKey, req.LogType, req.Logs)
		return publishLogsResponse{
			Message:     message,
			ErrorCode:   errcode,
			NodeInvalid: valid,
			Err:         err,
		}, nil
	}
}

// PublishLogs implements KolideService.PublishLogs
func (e Endpoints) PublishLogs(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
	newCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request := logCollection{NodeKey: nodeKey, LogType: logType, Logs: logs}
	response, err := e.PublishLogsEndpoint(newCtx, request)
	if err != nil {
		return "", "", false, err
	}
	resp := response.(publishLogsResponse)
	return resp.Message, resp.ErrorCode, resp.NodeInvalid, resp.Err
}

func (s *grpcServer) PublishLogs(ctx context.Context, req *pb.LogCollection) (*pb.AgentApiResponse, error) {
	_, rep, err := s.logs.ServeGRPC(ctx, req)
	if err != nil {
		return nil, err
	}
	return rep.(*pb.AgentApiResponse), nil
}

func (mw logmw) PublishLogs(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (message, errcode string, reauth bool, err error) {
	defer func(begin time.Time) {
		uuid, _ := uuid.FromContext(ctx)

		if message == "" {
			if err == nil {
				message = "success"
			} else {
				message = "failure"
			}
		}

		pubStateVals, ok := ctx.Value(PublicationCtxKey).(map[string]int)
		if !ok {
			pubStateVals = make(map[string]int)
		}

		mw.knapsack.Slogger().Log(ctx, levelForError(err), message, // nolint:sloglint // it's fine to not have a constant or literal here
			"method", "PublishLogs",
			"uuid", uuid,
			"logType", logType,
			"log_count", len(logs),
			"errcode", errcode,
			"reauth", reauth,
			"err", err,
			"took", time.Since(begin),
			"publication_state", pubStateVals,
		)
	}(time.Now())

	message, errcode, reauth, err = mw.next.PublishLogs(ctx, nodeKey, logType, logs)
	return message, errcode, reauth, err
}

func (mw uuidmw) PublishLogs(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (message, errcode string, reauth bool, err error) {
	ctx = uuid.NewContext(ctx, uuid.NewForRequest())
	return mw.next.PublishLogs(ctx, nodeKey, logType, logs)
}
