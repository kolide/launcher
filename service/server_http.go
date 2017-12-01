package service

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
	kithttp "github.com/go-kit/kit/transport/http"
	"github.com/gorilla/mux"
)

func NewHTTPHandler(e KolideClient, logger log.Logger) http.Handler {
	opts := []kithttp.ServerOption{
		kithttp.ServerBefore(kithttp.PopulateRequestContext),
		kithttp.ServerErrorLogger(logger),
		kithttp.ServerErrorEncoder(encodeError),
	}

	newServer := func(e endpoint.Endpoint, decodeFn kithttp.DecodeRequestFunc) http.Handler {
		return kithttp.NewServer(e, decodeFn, encodeResponse, opts...)
	}

	enrollHandler := newServer(
		e.RequestEnrollmentEndpoint,
		decodeHTTPEnrollmentRequest,
	)

	configHandler := newServer(
		e.RequestConfigEndpoint,
		decodeHTTPLauncherRequest,
	)

	logHandler := newServer(
		e.PublishLogsEndpoint,
		decodeHTTPLogCollectionRequest,
	)

	queriesHandler := newServer(
		e.RequestQueriesEndpoint,
		decodeHTTPLauncherRequest,
	)

	resultsHandler := newServer(
		e.PublishResultsEndpoint,
		decodeHTTPResultCollectionRequest,
	)

	r := mux.NewRouter()
	r.Handle("/api/v1/launcher/enroll", enrollHandler).
		Methods("POST").
		Name("enroll_launcher")

	r.Handle("/api/v1/launcher/config", configHandler).
		Methods("POST").
		Name("launcher_config")

	r.Handle("/api/v1/launcher/log", logHandler).
		Methods("POST").
		Name("launcher_log")

	r.Handle("/api/v1/launcher/distributed/read", queriesHandler).
		Methods("POST").
		Name("launcher_request_queries")

	r.Handle("/api/v1/launcher/distributed/write", resultsHandler).
		Methods("POST").
		Name("launcher_write_query_results")

	return r
}

// erroer interface is implemented by response structs to encode business logic errors
type errorer interface {
	error() error
}

func encodeResponse(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	if e, ok := response.(errorer); ok && e.error() != nil {
		encodeError(ctx, e.error(), w)
		return nil
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(response)
}

func encodeError(ctx context.Context, err error, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	w.WriteHeader(http.StatusInternalServerError)
	enc.Encode(map[string]interface{}{
		"error": err.Error(),
	})
}
