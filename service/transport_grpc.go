package service

import (
	"context"

	"github.com/kolide/launcher/service/internal/launcherproto"
)

func decodeGRPCAgentAPIRequest(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*kolide_agent.AgentApiRequest)
	return agentAPIRequest{
		NodeKey: req.NodeKey,
	}, nil
}

func encodeGRPCAgentAPIRequest(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(agentAPIRequest)
	return &kolide_agent.AgentApiRequest{
		NodeKey: req.NodeKey,
	}, nil
}

func decodeGRPCAgentAPIResponse(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*kolide_agent.AgentApiResponse)
	return agentAPIResponse{
		Message:     req.Message,
		ErrorCode:   req.ErrorCode,
		NodeInvalid: req.NodeInvalid,
	}, nil
}

func encodeGRPCAgentAPIResponse(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(agentAPIResponse)
	return &kolide_agent.AgentApiResponse{
		Message:     req.Message,
		ErrorCode:   req.ErrorCode,
		NodeInvalid: req.NodeInvalid,
	}, nil
}
