package service

import (
	"context"

	"github.com/go-kit/kit/endpoint"
	"github.com/kolide/osquery-go/plugin/distributed"
	"github.com/kolide/osquery-go/plugin/logger"
)

// KolideClient is the implementation of the KolideService interface, intended
// to be used as an API client.
type KolideClient struct {
	RequestEnrollmentEndpoint endpoint.Endpoint
	RequestConfigEndpoint     endpoint.Endpoint
	PublishLogsEndpoint       endpoint.Endpoint
	RequestQueriesEndpoint    endpoint.Endpoint
	PublishResultsEndpoint    endpoint.Endpoint
}

type enrollmentRequest struct {
	EnrollSecret   string
	HostIdentifier string
}

type enrollmentResponse struct {
	NodeKey     string
	NodeInvalid bool
	Err         error
}

// RequestEnrollment implements KolideService.RequestEnrollment
func (e KolideClient) RequestEnrollment(ctx context.Context, enrollSecret, hostIdentifier string) (string, bool, error) {
	request := enrollmentRequest{EnrollSecret: enrollSecret, HostIdentifier: hostIdentifier}
	response, err := e.RequestEnrollmentEndpoint(ctx, request)
	if err != nil {
		return "", false, err
	}
	resp := response.(enrollmentResponse)
	return resp.NodeKey, resp.NodeInvalid, resp.Err
}

type agentAPIRequest struct {
	NodeKey      string
	AgentVersion string
}

type configResponse struct {
	ConfigJSONBlob string
	NodeInvalid    bool
	Err            error
}

// RequestConfig implements KolideService.RequestConfig.
func (e KolideClient) RequestConfig(ctx context.Context, nodeKey, version string) (string, bool, error) {
	request := agentAPIRequest{NodeKey: nodeKey, AgentVersion: version}
	response, err := e.RequestConfigEndpoint(ctx, request)
	if err != nil {
		return "", false, err
	}
	resp := response.(configResponse)
	return resp.ConfigJSONBlob, resp.NodeInvalid, resp.Err
}

type logCollection struct {
	NodeKey      string
	AgentVersion string
	LogType      logger.LogType
	Logs         []string
}

type agentAPIResponse struct {
	Message     string
	ErrorCode   string
	NodeInvalid bool
	Err         error
}

// PublishLogs implements KolideService.PublishLogs
func (e KolideClient) PublishLogs(ctx context.Context, nodeKey, version string, logType logger.LogType, logs []string) (string, string, bool, error) {
	request := logCollection{NodeKey: nodeKey, AgentVersion: version, LogType: logType, Logs: logs}
	response, err := e.PublishLogsEndpoint(ctx, request)
	if err != nil {
		return "", "", false, err
	}
	resp := response.(agentAPIResponse)
	return resp.Message, resp.ErrorCode, resp.NodeInvalid, resp.Err
}

type queryCollection struct {
	Queries     distributed.GetQueriesResult
	NodeInvalid bool
	Err         error
}

// RequestQueries implements KolideService.RequestQueries
func (e KolideClient) RequestQueries(ctx context.Context, nodeKey, version string) (*distributed.GetQueriesResult, bool, error) {
	request := agentAPIRequest{NodeKey: nodeKey, AgentVersion: version}
	response, err := e.RequestQueriesEndpoint(ctx, request)
	if err != nil {
		return nil, false, err
	}
	resp := response.(queryCollection)
	return &resp.Queries, resp.NodeInvalid, resp.Err
}

type resultCollection struct {
	NodeKey string
	Results []distributed.Result
}

// PublishResults implements KolideService.PublishResults
func (e KolideClient) PublishResults(ctx context.Context, nodeKey string, results []distributed.Result) (string, string, bool, error) {
	request := resultCollection{NodeKey: nodeKey, Results: results}
	response, err := e.PublishResultsEndpoint(ctx, request)
	if err != nil {
		return "", "", false, err
	}
	resp := response.(agentAPIResponse)
	return resp.Message, resp.ErrorCode, resp.NodeInvalid, resp.Err
}
