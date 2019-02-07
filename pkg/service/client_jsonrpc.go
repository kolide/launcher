package service

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/url"

	"github.com/go-kit/kit/log"
	jsonrpc "github.com/go-kit/kit/transport/http/jsonrpc"
)

// New creates a new Kolide Client (implementation of the KolideService
// interface) using the provided gRPC client connection.
func NewJSONRPC(
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

	requestEnrollmentEndpoint := jsonrpc.NewClient(
		serviceURL,
		"RequestEnrollment",
		jsonrpc.SetClient(httpClient),
	).Endpoint()

	requestConfigEndpoint := jsonrpc.NewClient(
		serviceURL,
		"RequestConfig",
		jsonrpc.SetClient(httpClient),
	).Endpoint()

	publishLogsEndpoint := jsonrpc.NewClient(
		serviceURL,
		"PublishLogs",
		jsonrpc.SetClient(httpClient),
	).Endpoint()

	requestQueriesEndpoint := jsonrpc.NewClient(
		serviceURL,
		"RequestQueries",
		jsonrpc.SetClient(httpClient),
	).Endpoint()

	publishResultsEndpoint := jsonrpc.NewClient(
		serviceURL,
		"PublishResults",
		jsonrpc.SetClient(httpClient),
	).Endpoint()

	checkHealthEndpoint := jsonrpc.NewClient(
		serviceURL,
		"CheckHealth",
		jsonrpc.SetClient(httpClient),
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
