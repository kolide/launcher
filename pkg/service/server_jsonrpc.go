package service

import (
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/transport/http/jsonrpc"
)

func NewJSONRPCServer(endpoints Endpoints, logger log.Logger) *jsonrpc.Server {
	handler := jsonrpc.NewServer(
		makeEndpointCodecMap(endpoints),
		jsonrpc.ServerErrorLogger(logger),
	)
	return handler
}

// makeEndpointCodecMap returns a codec map configured.
func makeEndpointCodecMap(endpoints Endpoints) jsonrpc.EndpointCodecMap {
	return jsonrpc.EndpointCodecMap{
		"RequestEnrollment": jsonrpc.EndpointCodec{
			Endpoint: endpoints.RequestEnrollmentEndpoint,
			Decode:   decodeJSONRPCEnrollmentRequest,
			Encode:   encodeJSONRPCEnrollmentResponse,
		},
		"RequestConfig": jsonrpc.EndpointCodec{
			Endpoint: endpoints.RequestConfigEndpoint,
			Decode:   decodeJSONRPCConfigRequest,
			Encode:   encodeJSONRPCConfigResponse,
		},
		"RequestQueries": jsonrpc.EndpointCodec{
			Endpoint: endpoints.RequestQueriesEndpoint,
			Decode:   decodeJSONRPCQueriesRequest,
			Encode:   encodeJSONRPCQueryCollection,
		},
		"PublishLogs": jsonrpc.EndpointCodec{
			Endpoint: endpoints.PublishLogsEndpoint,
			Decode:   decodeJSONRPCLogCollection,
			Encode:   encodeJSONRPCPublishLogsResponse,
		},
		"PublishResults": jsonrpc.EndpointCodec{
			Endpoint: endpoints.PublishResultsEndpoint,
			Decode:   decodeJSONRPCResultCollection,
			Encode:   encodeJSONRPCPublishResultsResponse,
		},
		"CheckHealth": jsonrpc.EndpointCodec{
			Endpoint: endpoints.CheckHealthEndpoint,
			Decode:   decodeJSONRPCHealthCheckRequest,
			Encode:   encodeJSONRPCHealthcheckResponse,
		},
	}

}
