package service

import (
	"context"
	"crypto/tls"
	"github.com/go-kit/kit/log"
	"net/http"

	twirptransport "github.com/go-kit/kit/transport/twirp"

	pb "github.com/kolide/launcher/service/internal/launcherproto"
	"github.com/kolide/launcher/service/uuid"
)

func attachUUIDHeaderTwirp() twirptransport.ClientOption {
	return twirptransport.ClientBefore(
		func(ctx context.Context) (context.Context, error) {
			uuid, _ := uuid.FromContext(ctx)
			return twirptransport.SetRequestHeader("uuid", uuid)(ctx)
		},
	)
}

// New creates a new KolideClient (implementation of the KolideService
// interface) using the generated Twirp client.
func NewTwirpHTTPClient(kolideServerURL string, insecureTLS bool, logger log.Logger) KolideService {
	httpClient := http.DefaultClient
	if insecureTLS {
		httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
	}

	apiClient := pb.NewApiProtobufClient(kolideServerURL, httpClient)

	requestEnrollmentEndpoint := twirptransport.NewClient(
		func(ctx context.Context, request interface{}) (interface{}, error) {
			req := request.(*pb.EnrollmentRequest)
			return apiClient.RequestEnrollment(ctx, req)
		},
		encodeProtobufEnrollmentRequest,
		decodeProtobufEnrollmentResponse,
		attachUUIDHeaderTwirp(),
	).Endpoint()

	requestConfigEndpoint := twirptransport.NewClient(
		func(ctx context.Context, request interface{}) (interface{}, error) {
			req := request.(*pb.AgentApiRequest)
			return apiClient.RequestConfig(ctx, req)
		},
		encodeProtobufAgentAPIRequest,
		decodeProtobufConfigResponse,
		attachUUIDHeaderTwirp(),
	).Endpoint()

	publishLogsEndpoint := twirptransport.NewClient(
		func(ctx context.Context, request interface{}) (interface{}, error) {
			req := request.(*pb.LogCollection)
			return apiClient.PublishLogs(ctx, req)
		},
		encodeProtobufLogCollection,
		decodeProtobufAgentAPIResponse,
		attachUUIDHeaderTwirp(),
	).Endpoint()

	requestQueriesEndpoint := twirptransport.NewClient(
		func(ctx context.Context, request interface{}) (interface{}, error) {
			req := request.(*pb.AgentApiRequest)
			return apiClient.RequestQueries(ctx, req)
		},
		encodeProtobufAgentAPIRequest,
		decodeProtobufQueryCollection,
		attachUUIDHeaderTwirp(),
	).Endpoint()

	publishResultsEndpoint := twirptransport.NewClient(
		func(ctx context.Context, request interface{}) (interface{}, error) {
			req := request.(*pb.ResultCollection)
			return apiClient.PublishResults(ctx, req)
		},
		encodeProtobufResultCollection,
		decodeProtobufAgentAPIResponse,
		attachUUIDHeaderTwirp(),
	).Endpoint()

	checkHealthEndpoint := twirptransport.NewClient(
		func(ctx context.Context, request interface{}) (interface{}, error) {
			req := request.(*pb.AgentApiRequest)
			return apiClient.CheckHealth(ctx, req)
		},
		encodeProtobufAgentAPIRequest,
		decodeProtobufHealthCheckResponse,
		attachUUIDHeaderTwirp(),
	).Endpoint()

	var client KolideService = KolideClient{
		RequestEnrollmentEndpoint: requestEnrollmentEndpoint,
		RequestConfigEndpoint:     requestConfigEndpoint,
		PublishLogsEndpoint:       publishLogsEndpoint,
		RequestQueriesEndpoint:    requestQueriesEndpoint,
		PublishResultsEndpoint:    publishResultsEndpoint,
		CheckHealthEndpoint:       checkHealthEndpoint,
	}

	client = LoggingMiddleware(logger)(client)
	// Wrap with UUID middleware after logger so that UUID is available in
	// the logger context.
	client = uuidMiddleware(client)

	return client
}
