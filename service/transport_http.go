package service

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/pkg/errors"
)

func decodeHTTPEnrollmentRequest(ctx context.Context, r *http.Request) (interface{}, error) {
	var req enrollmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, errors.Wrap(err, "decode enrollment request")
	}
	return req, nil
}

func decodeHTTPLauncherRequest(ctx context.Context, r *http.Request) (interface{}, error) {
	var req agentAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, errors.Wrap(err, "decode agent_api request")
	}
	return req, nil
}

func decodeHTTPLogCollectionRequest(ctx context.Context, r *http.Request) (interface{}, error) {
	var req logCollection
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, errors.Wrap(err, "decode log collection request")
	}
	return req, nil
}

func decodeHTTPResultCollectionRequest(ctx context.Context, r *http.Request) (interface{}, error) {
	var req resultCollection
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, errors.Wrap(err, "decode result collection request")
	}
	return req, nil
}

func decodeHTTPEnrollmentResponse(_ context.Context, r *http.Response) (interface{}, error) {
	if r.StatusCode != http.StatusOK {
		return nil, errorDecoder(r)
	}
	var resp enrollmentResponse
	err := json.NewDecoder(r.Body).Decode(&resp)
	return resp, errors.Wrap(err, "decode enrollment response")
}

func decodeHTTPConfigResponse(_ context.Context, r *http.Response) (interface{}, error) {
	if r.StatusCode != http.StatusOK {
		return nil, errorDecoder(r)
	}
	var resp configResponse
	err := json.NewDecoder(r.Body).Decode(&resp)
	return resp, errors.Wrap(err, "decode config response")
}

func decodeHTTPQueryCollectionResponse(_ context.Context, r *http.Response) (interface{}, error) {
	if r.StatusCode != http.StatusOK {
		return nil, errorDecoder(r)
	}
	var resp queryCollection
	err := json.NewDecoder(r.Body).Decode(&resp)
	return resp, errors.Wrap(err, "decode query collection response")
}

func decodeLauncherResponse(_ context.Context, r *http.Response) (interface{}, error) {
	if r.StatusCode != http.StatusOK {
		return nil, errorDecoder(r)
	}
	var resp agentAPIResponse
	err := json.NewDecoder(r.Body).Decode(&resp)
	return resp, errors.Wrap(err, "decode agent_api response")
}

func errorDecoder(r *http.Response) error {
	var w errorWrapper
	if err := json.NewDecoder(r.Body).Decode(&w); err != nil {
		return err
	}
	return errors.New(w.Error)
}

type errorWrapper struct {
	Error string `json:"error"`
}
