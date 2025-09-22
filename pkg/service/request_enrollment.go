package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/transport/http/jsonrpc"
	"github.com/kolide/kit/contexts/uuid"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
)

type enrollmentRequest struct {
	EnrollSecret      string `json:"enroll_secret"`
	HostIdentifier    string `json:"host_identifier"`
	EnrollmentDetails EnrollmentDetails
}

type EnrollmentDetails = types.EnrollmentDetails

type enrollmentResponse struct {
	jsonRpcResponse
	NodeKey     string `json:"node_key"`
	NodeInvalid bool   `json:"node_invalid"`
	ErrorCode   string `json:"error_code,omitempty"`
	Err         error  `json:"err,omitempty"`
}

func decodeJSONRPCEnrollmentRequest(_ context.Context, msg json.RawMessage) (interface{}, error) {
	var req enrollmentRequest

	if err := json.Unmarshal(msg, &req); err != nil {
		return nil, &jsonrpc.Error{
			Code:    -32000,
			Message: fmt.Sprintf("couldn't unmarshal body to enrollment request: %s", err),
		}
	}
	return req, nil
}

func decodeJSONRPCEnrollmentResponse(_ context.Context, res jsonrpc.Response) (interface{}, error) {
	if res.Error != nil {
		return nil, *res.Error
	}
	var result enrollmentResponse
	err := json.Unmarshal(res.Result, &result)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling RequestEnrollment response: %w", err)
	}

	return result, nil
}

func encodeJSONRPCEnrollmentResponse(_ context.Context, obj interface{}) (json.RawMessage, error) {
	res, ok := obj.(enrollmentResponse)
	if !ok {
		return encodeJSONResponse(nil, fmt.Errorf("asserting result to *enrollmentResponse failed. Got %T, %+v", obj, obj))
	}

	b, err := json.Marshal(res)
	if err != nil {
		return encodeJSONResponse(b, fmt.Errorf("marshal json response: %w", err))
	}

	return encodeJSONResponse(b, nil)
}

func MakeRequestEnrollmentEndpoint(svc KolideService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(enrollmentRequest)
		nodeKey, valid, err := svc.RequestEnrollment(ctx, req.EnrollSecret, req.HostIdentifier, req.EnrollmentDetails)
		return enrollmentResponse{
			NodeKey:     nodeKey,
			NodeInvalid: valid,
			Err:         err,
		}, nil
	}
}

// requestTimeout is duration after which the request is cancelled.
const requestTimeout = 60 * time.Second

// RequestEnrollment implements KolideService.RequestEnrollment
func (e Endpoints) RequestEnrollment(ctx context.Context, enrollSecret, hostIdentifier string, details EnrollmentDetails) (string, bool, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	newCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	request := enrollmentRequest{EnrollSecret: enrollSecret, HostIdentifier: hostIdentifier, EnrollmentDetails: details}
	response, err := e.RequestEnrollmentEndpoint(newCtx, request)

	if err != nil {
		return "", false, err
	}
	resp := response.(enrollmentResponse)

	if resp.DisableDevice {
		return "", false, ErrDeviceDisabled{}
	}

	return resp.NodeKey, resp.NodeInvalid, resp.Err
}

func (mw logmw) RequestEnrollment(ctx context.Context, enrollSecret, hostIdentifier string, details EnrollmentDetails) (nodekey string, reauth bool, err error) {
	defer func(begin time.Time) {
		uuid, _ := uuid.FromContext(ctx)

		message := "success"
		if err != nil {
			message = "failure requesting enrollment"
		}

		keyvals := []interface{}{
			"method", "RequestEnrollment",
			"uuid", uuid,
			"hostIdentifier", hostIdentifier,
			"reauth", reauth,
			"err", err,
			"took", time.Since(begin),
		}

		if err != nil {
			keyvals = append(keyvals,
				"enrollSecret", enrollSecret,
				"nodekey", nodekey,
			)
		}

		mw.knapsack.Slogger().Log(ctx, levelForError(err), message, keyvals...) // nolint:sloglint // it's fine to not have a constant or literal here
	}(time.Now())

	nodekey, reauth, err = mw.next.RequestEnrollment(ctx, enrollSecret, hostIdentifier, details)
	return nodekey, reauth, err
}

func (mw uuidmw) RequestEnrollment(ctx context.Context, enrollSecret, hostIdentifier string, details EnrollmentDetails) (errcode string, reauth bool, err error) {
	ctx = uuid.NewContext(ctx, uuid.NewForRequest())
	return mw.next.RequestEnrollment(ctx, enrollSecret, hostIdentifier, details)
}
