package service

import (
	"context"
	"net/http"
	"net/url"

	"github.com/go-kit/kit/log"
	httptransport "github.com/go-kit/kit/transport/http"
	"github.com/kolide/launcher/service/uuid"
)

func attachUUIDHeaderHTTP() httptransport.ClientOption {
	return httptransport.ClientBefore(
		func(ctx context.Context, r *http.Request) context.Context {
			uuid, _ := uuid.FromContext(ctx)
			return httptransport.SetRequestHeader("uuid", uuid)(ctx, r)
		},
	)
}

// NewHTTPClient creates an HTTP Client that implements the Launcher endpoints.
func NewHTTPClient(instance string, logger log.Logger) (KolideService, error) {
	u, err := url.Parse(instance)
	if err != nil {
		return nil, err
	}

	opts := []httptransport.ClientOption{
		attachUUIDHeaderHTTP(),
	}

	requestEnrollmentEndpoint := httptransport.NewClient(
		"POST",
		copyURL(u, "/api/v1/launcher/enroll"),
		httptransport.EncodeJSONRequest,
		decodeHTTPEnrollmentResponse,
		opts...,
	).Endpoint()

	requestConfigEndpoint := httptransport.NewClient(
		"POST",
		copyURL(u, "/api/v1/launcher/config"),
		httptransport.EncodeJSONRequest,
		decodeHTTPConfigResponse,
		opts...,
	).Endpoint()

	publishLogsEndpoint := httptransport.NewClient(
		"POST",
		copyURL(u, "/api/v1/launcher/log"),
		httptransport.EncodeJSONRequest,
		decodeLauncherResponse,
		opts...,
	).Endpoint()

	requestQueriesEndpoint := httptransport.NewClient(
		"POST",
		copyURL(u, "/api/v1/launcher/distributed/read"),
		httptransport.EncodeJSONRequest,
		decodeHTTPQueryCollectionResponse,
		opts...,
	).Endpoint()

	publishResultsEndpoint := httptransport.NewClient(
		"POST",
		copyURL(u, "/api/v1/launcher/distributed/write"),
		httptransport.EncodeJSONRequest,
		decodeLauncherResponse,
		opts...,
	).Endpoint()

	var client KolideService = KolideClient{
		RequestEnrollmentEndpoint: requestEnrollmentEndpoint,
		RequestConfigEndpoint:     requestConfigEndpoint,
		PublishLogsEndpoint:       publishLogsEndpoint,
		RequestQueriesEndpoint:    requestQueriesEndpoint,
		PublishResultsEndpoint:    publishResultsEndpoint,
	}

	client = LoggingMiddleware(logger)(client)
	client = uuidMiddleware(client)

	return client, nil

}

func copyURL(base *url.URL, path string) *url.URL {
	next := *base
	next.Path = path
	return &next
}
