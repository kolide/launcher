package service

import (
	"crypto/x509"
	"net/http"
	"net/url"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/transport/http/jsonrpc"
)

// New creates a new Kolide Client (implementation of the KolideService
// interface) using the provided gRPC client connection.
func NewJSONRPCClient(
	serverURL string,
	insecureTLS bool,
	certPins [][]byte,
	rootPool *x509.CertPool,
	logger log.Logger,
) KolideService {
	serviceURL := &url.URL{
		Scheme: "https",
		Host:   serverURL,
	}

	tlsConfig := makeTLSConfig(serverURL, insecureTLS, certPins, rootPool, logger)
	httpClient := http.DefaultClient
	httpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	requestEnrollmentEndpoint := jsonrpc.NewClient(
		serviceURL,
		"RequestEnrollment",
		jsonrpc.SetClient(httpClient),
		jsonrpc.ClientResponseDecoder(decodeJSONRPCEnrollmentResponse),
	).Endpoint()

	requestConfigEndpoint := jsonrpc.NewClient(
		serviceURL,
		"RequestConfig",
		jsonrpc.SetClient(httpClient),
		jsonrpc.ClientResponseDecoder(decodeJSONRPCConfigResponse),
	).Endpoint()

	publishLogsEndpoint := jsonrpc.NewClient(
		serviceURL,
		"PublishLogs",
		jsonrpc.SetClient(httpClient),
		jsonrpc.ClientResponseDecoder(decodeJSONRPCPublishLogsResponse),
	).Endpoint()

	requestQueriesEndpoint := jsonrpc.NewClient(
		serviceURL,
		"RequestQueries",
		jsonrpc.SetClient(httpClient),
		jsonrpc.ClientResponseDecoder(decodeJSONRPCQueryCollection),
	).Endpoint()

	publishResultsEndpoint := jsonrpc.NewClient(
		serviceURL,
		"PublishResults",
		jsonrpc.SetClient(httpClient),
		jsonrpc.ClientResponseDecoder(decodeJSONRPCPublishResultsResponse),
	).Endpoint()

	checkHealthEndpoint := jsonrpc.NewClient(
		serviceURL,
		"CheckHealth",
		jsonrpc.SetClient(httpClient),
		jsonrpc.ClientResponseDecoder(decodeJSONRPCHealthCheckResponse),
	).Endpoint()

	var client KolideService = Endpoints{
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
