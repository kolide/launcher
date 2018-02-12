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
		func(ctx context.Context, h *http.Header) context.Context {
			uuid, _ := uuid.FromContext(ctx)
			return twirptransport.SetRequestHeader("uuid", uuid)(ctx, h)
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
		apiClient,
		"RequestEnrollment",
		encodeProtobufEnrollmentRequest,
		decodeProtobufEnrollmentResponse,
		attachUUIDHeaderTwirp(),
	).Endpoint()

	requestConfigEndpoint := twirptransport.NewClient(
		apiClient,
		"RequestConfig",
		encodeProtobufAgentAPIRequest,
		decodeProtobufConfigResponse,
		attachUUIDHeaderTwirp(),
	).Endpoint()

	publishLogsEndpoint := twirptransport.NewClient(
		apiClient,
		"PublishLogs",
		encodeProtobufLogCollection,
		decodeProtobufAgentAPIResponse,
		attachUUIDHeaderTwirp(),
	).Endpoint()

	requestQueriesEndpoint := twirptransport.NewClient(
		apiClient,
		"RequestQueries",
		encodeProtobufAgentAPIRequest,
		decodeProtobufQueryCollection,
		attachUUIDHeaderTwirp(),
	).Endpoint()

	publishResultsEndpoint := twirptransport.NewClient(
		apiClient,
		"PublishResults",
		encodeProtobufResultCollection,
		decodeProtobufAgentAPIResponse,
		attachUUIDHeaderTwirp(),
	).Endpoint()

	checkHealthEndpoint := twirptransport.NewClient(
		apiClient,
		"CheckHealth",
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
