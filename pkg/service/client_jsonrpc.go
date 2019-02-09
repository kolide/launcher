package service

import (
	"bytes"
	"context"
	"crypto/x509"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/go-kit/kit/log"
	kithttp "github.com/go-kit/kit/transport/http"
	"github.com/go-kit/kit/transport/http/jsonrpc"
)

// forceNoChunkedEncoding forces the connection not to use chunked
// encoding. This is because we're talking to rails wehich doeasn't
// support it. TOD: followup info
func forceNoChunkedEncoding(ctx context.Context, r *http.Request) context.Context {
	r.TransferEncoding = []string{"identity"}

	// read the body, set the content legth, and leave a new ReadCloser in Body
	bodyBuf := &bytes.Buffer{}
	bodyReadBytes, _ := io.Copy(bodyBuf, r.Body)
	r.ContentLength = bodyReadBytes
	r.Body = ioutil.NopCloser(bodyBuf)

	return ctx
}

// New creates a new Kolide Client (implementation of the KolideService
// interface) using the provided gRPC client connection.
func NewJSONRPCClient(
	serverURL string,
	insecureTLS bool,
	insecureJSONRPC bool,
	certPins [][]byte,
	rootPool *x509.CertPool,
	logger log.Logger,
) KolideService {
	serviceURL := &url.URL{
		Scheme: "https",
		Host:   serverURL,
	}

	if insecureJSONRPC {
		serviceURL.Scheme = "http"
	}

	tlsConfig := makeTLSConfig(serverURL, insecureTLS, certPins, rootPool, logger)
	httpClient := http.DefaultClient
	httpClient = &http.Client{
		Timeout: time.Second * 30,
		Transport: &http.Transport{
			TLSClientConfig:   tlsConfig,
			DisableKeepAlives: true,
		},
	}

	commonOpts := []jsonrpc.ClientOption{
		jsonrpc.SetClient(httpClient),
		jsonrpc.ClientBefore(
			forceNoChunkedEncoding,
			//kithttp.SetRequestHeader("Transfer-Encoding", "identity"),
			kithttp.SetRequestHeader("x-seph", "test"),
		),
	}

	requestEnrollmentEndpoint := jsonrpc.NewClient(
		serviceURL,
		"RequestEnrollment",
		append(commonOpts, jsonrpc.ClientResponseDecoder(decodeJSONRPCEnrollmentResponse))...,
	).Endpoint()

	requestConfigEndpoint := jsonrpc.NewClient(
		serviceURL,
		"RequestConfig",
		append(commonOpts, jsonrpc.ClientResponseDecoder(decodeJSONRPCConfigResponse))...,
	).Endpoint()

	publishLogsEndpoint := jsonrpc.NewClient(
		serviceURL,
		"PublishLogs",
		append(commonOpts, jsonrpc.ClientResponseDecoder(decodeJSONRPCPublishLogsResponse))...,
	).Endpoint()

	requestQueriesEndpoint := jsonrpc.NewClient(
		serviceURL,
		"RequestQueries",
		append(commonOpts, jsonrpc.ClientResponseDecoder(decodeJSONRPCQueryCollection))...,
	).Endpoint()

	publishResultsEndpoint := jsonrpc.NewClient(
		serviceURL,
		"PublishResults",
		append(commonOpts, jsonrpc.ClientResponseDecoder(decodeJSONRPCPublishResultsResponse))...,
	).Endpoint()

	checkHealthEndpoint := jsonrpc.NewClient(
		serviceURL,
		"CheckHealth",
		append(commonOpts, jsonrpc.ClientResponseDecoder(decodeJSONRPCHealthCheckResponse))...,
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
