package service

import (
	"context"
	"time"

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
	RequestPracticesEndpoint  endpoint.Endpoint
	PublishResultsEndpoint    endpoint.Endpoint
	CheckHealthEndpoint       endpoint.Endpoint
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

// requestTimeout is duration after which the request is cancelled.
const requestTimeout = 60 * time.Second

// RequestEnrollment implements KolideService.RequestEnrollment
func (e KolideClient) RequestEnrollment(ctx context.Context, enrollSecret, hostIdentifier string) (string, bool, error) {
	newCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	request := enrollmentRequest{EnrollSecret: enrollSecret, HostIdentifier: hostIdentifier}
	response, err := e.RequestEnrollmentEndpoint(newCtx, request)
	if err != nil {
		return "", false, err
	}
	resp := response.(enrollmentResponse)
	return resp.NodeKey, resp.NodeInvalid, resp.Err
}

type agentAPIRequest struct {
	NodeKey string
}

type configResponse struct {
	ConfigJSONBlob string
	NodeInvalid    bool
	Err            error
}

// RequestConfig implements KolideService.RequestConfig.
func (e KolideClient) RequestConfig(ctx context.Context, nodeKey string) (string, bool, error) {
	newCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request := agentAPIRequest{NodeKey: nodeKey}
	response, err := e.RequestConfigEndpoint(newCtx, request)
	if err != nil {
		return "", false, err
	}
	resp := response.(configResponse)
	return resp.ConfigJSONBlob, resp.NodeInvalid, resp.Err
}

type logCollection struct {
	NodeKey string
	LogType logger.LogType
	Logs    []string
}

type agentAPIResponse struct {
	Message     string
	ErrorCode   string
	NodeInvalid bool
	Err         error
}

// PublishLogs implements KolideService.PublishLogs
func (e KolideClient) PublishLogs(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
	newCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request := logCollection{NodeKey: nodeKey, LogType: logType, Logs: logs}
	response, err := e.PublishLogsEndpoint(newCtx, request)
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
func (e KolideClient) RequestQueries(ctx context.Context, nodeKey string) (*distributed.GetQueriesResult, bool, error) {
	newCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request := agentAPIRequest{NodeKey: nodeKey}
	response, err := e.RequestQueriesEndpoint(newCtx, request)
	if err != nil {
		return nil, false, err
	}
	resp := response.(queryCollection)
	return &resp.Queries, resp.NodeInvalid, resp.Err
}

// RequestPractices implements KolideService.RequestPractices
func (e KolideClient) RequestPractices(ctx context.Context, nodeKey string) (*distributed.GetQueriesResult, bool, error) {
	newCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request := agentAPIRequest{NodeKey: nodeKey}
	response, err := e.RequestPracticesEndpoint(newCtx, request)
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
	newCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	request := resultCollection{NodeKey: nodeKey, Results: results}
	response, err := e.PublishResultsEndpoint(newCtx, request)
	if err != nil {
		return "", "", false, err
	}
	resp := response.(agentAPIResponse)
	return resp.Message, resp.ErrorCode, resp.NodeInvalid, resp.Err
}

func (e KolideClient) CheckHealth(ctx context.Context) (int32, error) {
	newCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request := agentAPIRequest{}
	response, err := e.CheckHealthEndpoint(newCtx, request)
	if err != nil {
		return 0, err
	}
	resp := response.(healthcheckResponse)
	return resp.Status, resp.Err
}

type healthcheckResponse struct {
	Status int32
	Err    error
}

func makeRequestEnrollmentEndpoint(svc KolideService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(enrollmentRequest)
		nodeKey, valid, err := svc.RequestEnrollment(ctx, req.EnrollSecret, req.HostIdentifier)
		return enrollmentResponse{
			NodeKey:     nodeKey,
			NodeInvalid: valid,
			Err:         err,
		}, nil
	}
}

func makeRequestConfigEndpoint(svc KolideService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(agentAPIRequest)
		config, valid, err := svc.RequestConfig(ctx, req.NodeKey)
		return configResponse{
			ConfigJSONBlob: config,
			NodeInvalid:    valid,
			Err:            err,
		}, nil
	}
}

func makePublishLogsEndpoint(svc KolideService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(logCollection)
		message, errcode, valid, err := svc.PublishLogs(ctx, req.NodeKey, req.LogType, req.Logs)
		return agentAPIResponse{
			Message:     message,
			ErrorCode:   errcode,
			NodeInvalid: valid,
			Err:         err,
		}, nil
	}
}

func makeRequestQueriesEndpoint(svc KolideService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(agentAPIRequest)
		result, valid, err := svc.RequestQueries(ctx, req.NodeKey)
		if err != nil {
			return queryCollection{Err: err}, nil
		}
		return queryCollection{
			Queries:     *result,
			NodeInvalid: valid,
			Err:         err,
		}, nil
	}
}

func makeRequestPracticesEndpoint(svc KolideService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(agentAPIRequest)
		result, valid, err := svc.RequestPractices(ctx, req.NodeKey)
		if err != nil {
			return queryCollection{Err: err}, nil
		}
		return queryCollection{
			Queries:     *result,
			NodeInvalid: valid,
			Err:         err,
		}, nil
	}
}

func makePublishResultsEndpoint(svc KolideService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(resultCollection)
		message, errcode, valid, err := svc.PublishResults(ctx, req.NodeKey, req.Results)
		return agentAPIResponse{
			Message:     message,
			ErrorCode:   errcode,
			NodeInvalid: valid,
			Err:         err,
		}, nil
	}
}

func makeCheckHealthEndpoint(svc KolideService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		status, err := svc.CheckHealth(ctx)
		return healthcheckResponse{
			Status: status,
			Err:    err,
		}, nil
	}
}

func MakeServerEndpoints(svc KolideService) KolideClient {
	return KolideClient{
		RequestEnrollmentEndpoint: makeRequestEnrollmentEndpoint(svc),
		RequestConfigEndpoint:     makeRequestConfigEndpoint(svc),
		PublishLogsEndpoint:       makePublishLogsEndpoint(svc),
		RequestQueriesEndpoint:    makeRequestQueriesEndpoint(svc),
		RequestPracticesEndpoint:  makeRequestPracticesEndpoint(svc),
		PublishResultsEndpoint:    makePublishResultsEndpoint(svc),
		CheckHealthEndpoint:       makeCheckHealthEndpoint(svc),
	}
}
