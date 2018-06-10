package service

import (
	"context"
	"fmt"

	"github.com/go-kit/kit/endpoint"
	pb "github.com/kolide/launcher/service/internal/launcherproto"
	"github.com/kolide/osquery-go/plugin/logger"
	"github.com/pkg/errors"
)

type logCollection struct {
	NodeKey string
	LogType logger.LogType
	Logs    []string
}

type publishLogsResponse struct {
	Message     string
	ErrorCode   string
	NodeInvalid bool
	Err         error
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
		panic(fmt.Sprintf("logType %d not implemented", req.LogType))
	}

	return logCollection{
		NodeKey: req.NodeKey,
		LogType: typ,
		Logs:    logs,
	}, nil
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
	resp := response.(publishResultsResponse)
	return resp.Message, resp.ErrorCode, resp.NodeInvalid, resp.Err
}

func (s *grpcServer) PublishLogs(ctx context.Context, req *pb.LogCollection) (*pb.AgentApiResponse, error) {
	_, rep, err := s.logs.ServeGRPC(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "publish logs")
	}
	return rep.(*pb.AgentApiResponse), nil
}
