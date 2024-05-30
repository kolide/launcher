package service

import (
	"bytes"
	"context"
	"crypto/x509"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/go-kit/kit/transport/http/jsonrpc"
	"github.com/kolide/launcher/ee/agent/types"
)

// forceNoChunkedEncoding forces the connection not to use chunked
// encoding. It is designed as a go-kit httptransport.RequestFunc,
// suitable for being passed in with ClientBefore.  This is required
// when using rails as an endpoint, as rails does not support chunked
// encoding.
//
// It does this by calculating the body size, and applying a
// ContentLength header. This triggers http.client to not chunk it.
func forceNoChunkedEncoding(ctx context.Context, r *http.Request) context.Context {
	r.TransferEncoding = []string{"identity"}

	// read the body, set the content length, and leave a new ReadCloser in Body
	bodyBuf := &bytes.Buffer{}

	// We discard the error because we can't do anything about it here -- no logger access
	bodyReadBytes, _ := io.Copy(bodyBuf, r.Body)
	r.Body.Close()
	r.ContentLength = bodyReadBytes
	r.Body = io.NopCloser(bodyBuf)

	return ctx
}

type ErrDeviceDisabled struct{}

func (e ErrDeviceDisabled) Error() string {
	return "device disabled"
}

type jsonRpcResponse struct {
	DisableDevice bool `json:"disable_device"`
}

// New creates a new Kolide Client (implementation of the KolideService
// interface) using a JSONRPC client connection.
func NewJSONRPCClient(
	k types.Knapsack,
	rootPool *x509.CertPool,
	options ...jsonrpc.ClientOption,
) KolideService {
	serviceURL := &url.URL{
		Scheme: "https",
		Host:   k.KolideServerURL(),
	}

	if k.InsecureTransportTLS() {
		serviceURL.Scheme = "http"
	}

	httpClient := &http.Client{
		Timeout: time.Second * 30,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
	if !k.InsecureTransportTLS() {
		tlsConfig := makeTLSConfig(k, rootPool)
		httpClient.Transport = &http.Transport{
			TLSClientConfig:   tlsConfig,
			DisableKeepAlives: true,
		}
	}

	commonOpts := []jsonrpc.ClientOption{
		jsonrpc.SetClient(httpClient),
		jsonrpc.ClientBefore(
			forceNoChunkedEncoding,
		),
	}

	commonOpts = append(commonOpts, options...)

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

	client = LoggingMiddleware(k)(client)
	// Wrap with UUID middleware after logger so that UUID is available in
	// the logger context.
	client = uuidMiddleware(client)

	return client
}
