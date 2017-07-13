package service

import (
	"context"

	"github.com/kolide/agent-api"
	"github.com/kolide/osquery-go/plugin/distributed"
	"github.com/kolide/osquery-go/plugin/logger"
)

func DecodeGRPCEnrollmentRequest(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*kolide_agent.EnrollmentRequest)
	return enrollmentRequest{
		EnrollSecret:   req.EnrollSecret,
		HostIdentifier: req.HostIdentifier,
	}, nil
}

func EncodeGRPCEnrollmentRequest(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(enrollmentRequest)
	return &kolide_agent.EnrollmentRequest{
		EnrollSecret:   req.EnrollSecret,
		HostIdentifier: req.HostIdentifier,
	}, nil
}

func DecodeGRPCEnrollmentResponse(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*kolide_agent.EnrollmentResponse)
	return enrollmentResponse{
		NodeKey:     req.NodeKey,
		NodeInvalid: req.NodeInvalid,
	}, nil
}

func EncodeGRPCEnrollmentResponse(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(enrollmentResponse)
	return &kolide_agent.EnrollmentResponse{
		NodeKey:     req.NodeKey,
		NodeInvalid: req.NodeInvalid,
	}, nil
}

func DecodeGRPCAgentAPIRequest(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*kolide_agent.AgentApiRequest)
	return agentAPIRequest{
		NodeKey:      req.NodeKey,
		AgentVersion: req.AgentVersion,
	}, nil
}

func EncodeGRPCAgentAPIRequest(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(agentAPIRequest)
	return &kolide_agent.AgentApiRequest{
		NodeKey:      req.NodeKey,
		AgentVersion: req.AgentVersion,
	}, nil
}

func DecodeGRPCConfigResponse(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*kolide_agent.ConfigResponse)
	return configResponse{
		ConfigJSONBlob: req.ConfigJsonBlob,
		NodeInvalid:    req.NodeInvalid,
	}, nil
}

func EncodeGRPCConfigResponse(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(configResponse)
	return &kolide_agent.ConfigResponse{
		ConfigJsonBlob: req.ConfigJSONBlob,
		NodeInvalid:    req.NodeInvalid,
	}, nil
}

func DecodeGRPCAgentAPIResponse(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*kolide_agent.AgentApiResponse)
	return agentAPIResponse{
		Message:     req.Message,
		ErrorCode:   req.ErrorCode,
		NodeInvalid: req.NodeInvalid,
	}, nil
}

func EncodeGRPCAgentAPIResponse(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(agentAPIResponse)
	return &kolide_agent.AgentApiResponse{
		Message:     req.Message,
		ErrorCode:   req.ErrorCode,
		NodeInvalid: req.NodeInvalid,
	}, nil
}

func DecodeGRPCLogCollection(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*kolide_agent.LogCollection)
	logs := make([]string, 0, len(req.Logs))
	for _, log := range req.Logs {
		logs = append(logs, log.Data)
	}
	return logCollection{
		NodeKey: req.NodeKey,
		LogType: logger.LogType(req.LogType),
		Logs:    logs,
	}, nil
}

func EncodeGRPCLogCollection(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(logCollection)
	logs := make([]*kolide_agent.LogCollection_Log, 0, len(req.Logs))
	for _, log := range req.Logs {
		logs = append(logs, &kolide_agent.LogCollection_Log{log})
	}
	return &kolide_agent.LogCollection{
		NodeKey: req.NodeKey,
		LogType: kolide_agent.LogCollection_LogType(req.LogType),
		Logs:    logs,
	}, nil

}

func DecodeGRPCQueryCollection(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*kolide_agent.QueryCollection)
	queries := distributed.GetQueriesResult{
		Queries:   map[string]string{},
		Discovery: map[string]string{},
	}
	for _, query := range req.Queries {
		queries.Queries[query.Id] = query.Query
	}
	return queryCollection{
		Queries:     queries,
		NodeInvalid: req.NodeInvalid,
	}, nil
}

func EncodeGRPCQueryCollection(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(queryCollection)
	queries := make([]*kolide_agent.QueryCollection_Query, 0, len(req.Queries.Queries))
	for id, query := range req.Queries.Queries {
		queries = append(queries,
			&kolide_agent.QueryCollection_Query{
				Id:    id,
				Query: query,
			},
		)
	}
	return &kolide_agent.QueryCollection{
		Queries:     queries,
		NodeInvalid: req.NodeInvalid,
	}, nil
}

func DecodeGRPCResultCollection(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*kolide_agent.ResultCollection)

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

func EncodeGRPCResultCollection(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(resultCollection)

	results := make([]*kolide_agent.ResultCollection_Result, 0, len(req.Results))
	for _, result := range req.Results {
		// Iterate results
		rows := make([]*kolide_agent.ResultCollection_Result_ResultRow, 0, len(result.Rows))
		for _, row := range result.Rows {
			// Extract rows into columns
			rowCols := make([]*kolide_agent.ResultCollection_Result_ResultRow_Column, 0, len(row))
			for col, val := range row {
				rowCols = append(rowCols, &kolide_agent.ResultCollection_Result_ResultRow_Column{
					Name:  col,
					Value: val,
				})
			}
			rows = append(rows, &kolide_agent.ResultCollection_Result_ResultRow{rowCols})
		}
		results = append(results,
			&kolide_agent.ResultCollection_Result{
				Id:     result.QueryName,
				Status: int32(result.Status),
				Rows:   rows,
			},
		)
	}

	return &kolide_agent.ResultCollection{
		NodeKey: req.NodeKey,
		Results: results,
	}, nil
}
