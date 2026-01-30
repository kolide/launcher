package service

import (
	"bytes"
	"context"
	"crypto/x509"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/go-kit/kit/transport/http/jsonrpc"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
)

// forceNoChunkedEncoding forces the connection not to use chunked
// encoding. It is designed as a go-kit httptransport.RequestFunc,
// suitable for being passed in with ClientBefore.  This is required
// when using rails as an endpoint, as rails does not support chunked
// encoding.
//
// It does this by calculating the body size, and applying a
// ContentLength header. This triggers http.client to not chunk it.
func forceNoChunkedEncoding(slogger *slog.Logger) func(context.Context, *http.Request) context.Context {
	return func(ctx context.Context, r *http.Request) context.Context {
		r.TransferEncoding = []string{"identity"}

		// read the body, set the content length, and leave a new ReadCloser in Body
		bodyBuf := &bytes.Buffer{}

		bodyReadBytes, err := io.Copy(bodyBuf, r.Body)
		if err != nil {
			slogger.Log(ctx, slog.LevelError,
				"failed to copy request body for chunked encoding fix",
				"err", err,
			)
		}

		r.Body.Close()
		r.ContentLength = bodyReadBytes
		r.Body = io.NopCloser(bodyBuf)

		return ctx
	}
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
	serviceURL := buildServiceURL(k)

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
			forceNoChunkedEncoding(k.Slogger()),
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

	var client KolideService = &Endpoints{
		RequestEnrollmentEndpoint: requestEnrollmentEndpoint,
		RequestConfigEndpoint:     requestConfigEndpoint,
		PublishLogsEndpoint:       publishLogsEndpoint,
		RequestQueriesEndpoint:    requestQueriesEndpoint,
		PublishResultsEndpoint:    publishResultsEndpoint,
		CheckHealthEndpoint:       checkHealthEndpoint,
		endpointsLock:             &sync.RWMutex{},
		client:                    httpClient,
		k:                         k,
	}

	client = LoggingMiddleware(k)(client)
	// Wrap with UUID middleware after logger so that UUID is available in
	// the logger context.
	client = uuidMiddleware(client)

	k.RegisterChangeObserver(client, keys.KolideServerURL)

	return client
}

func buildServiceURL(k types.Knapsack) *url.URL {
	serviceURL := &url.URL{
		Scheme: "https",
		Host:   k.KolideServerURL(),
	}
	if k.InsecureTransportTLS() {
		serviceURL.Scheme = "http"
	}

	return serviceURL
}

func (e *Endpoints) FlagsChanged(ctx context.Context, flagKeys ...keys.FlagKey) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	if !slices.Contains(flagKeys, keys.KolideServerURL) {
		return
	}

	// Update URL, reusing previous http client
	serviceURL := buildServiceURL(e.k)
	opts := []jsonrpc.ClientOption{
		jsonrpc.SetClient(e.client),
		jsonrpc.ClientBefore(
			forceNoChunkedEncoding(e.k.Slogger()),
		),
	}

	// Re-create all endpoints with new base URL, locking to prevent concurrent requests during the update
	e.endpointsLock.Lock()
	defer e.endpointsLock.Unlock()
	e.RequestEnrollmentEndpoint = jsonrpc.NewClient(
		serviceURL,
		"RequestEnrollment",
		append(opts, jsonrpc.ClientResponseDecoder(decodeJSONRPCEnrollmentResponse))...,
	).Endpoint()

	e.RequestConfigEndpoint = jsonrpc.NewClient(
		serviceURL,
		"RequestConfig",
		append(opts, jsonrpc.ClientResponseDecoder(decodeJSONRPCConfigResponse))...,
	).Endpoint()

	e.PublishLogsEndpoint = jsonrpc.NewClient(
		serviceURL,
		"PublishLogs",
		append(opts, jsonrpc.ClientResponseDecoder(decodeJSONRPCPublishLogsResponse))...,
	).Endpoint()

	e.RequestQueriesEndpoint = jsonrpc.NewClient(
		serviceURL,
		"RequestQueries",
		append(opts, jsonrpc.ClientResponseDecoder(decodeJSONRPCQueryCollection))...,
	).Endpoint()

	e.PublishResultsEndpoint = jsonrpc.NewClient(
		serviceURL,
		"PublishResults",
		append(opts, jsonrpc.ClientResponseDecoder(decodeJSONRPCPublishResultsResponse))...,
	).Endpoint()

	e.CheckHealthEndpoint = jsonrpc.NewClient(
		serviceURL,
		"CheckHealth",
		append(opts, jsonrpc.ClientResponseDecoder(decodeJSONRPCHealthCheckResponse))...,
	).Endpoint()

	e.k.Slogger().Log(ctx, slog.LevelInfo,
		"successfully updated URL for Kolide service",
		"new_url", serviceURL.String(),
	)
}
