package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log/level"
	"github.com/go-kit/kit/transport/http/jsonrpc"
	"github.com/kolide/kit/contexts/uuid"
	"github.com/pkg/errors"

	pb "github.com/kolide/launcher/pkg/pb/launcher"
)

type enrollmentRequest struct {
	EnrollSecret      string `json:"enroll_secret"`
	HostIdentifier    string `json:"host_identifier"`
	EnrollmentDetails EnrollmentDetails
}

type EnrollmentDetails struct {
	OSVersion       string `json:"os_version"`
	OSBuildID       string `json:"os_build_id"`
	OSPlatform      string `json:"os_platform"`
	Hostname        string `json:"hostname"`
	HardwareVendor  string `json:"hardware_vendor"`
	HardwareModel   string `json:"hardware_model"`
	HardwareSerial  string `json:"hardware_serial"`
	OsqueryVersion  string `json:"osquery_version"`
	LauncherVersion string `json:"launcher_version"`
	OSName          string `json:"os_name"`
	OSPlatformLike  string `json:"os_platform_like"`
	GOOS            string `json:"goos"`
	GOARCH          string `json:"goarch"`
}

type enrollmentResponse struct {
	NodeKey     string `json:"node_key"`
	NodeInvalid bool   `json:"node_invalid"`
	ErrorCode   string `json:"error_code"`
	Err         error
}

func decodeGRPCEnrollmentRequest(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*pb.EnrollmentRequest)
	pbEnrollDetails := req.GetEnrollmentDetails()
	var enrollDetails EnrollmentDetails
	if pbEnrollDetails != nil {
		enrollDetails = EnrollmentDetails{
			OSVersion:       pbEnrollDetails.OsVersion,
			OSBuildID:       pbEnrollDetails.OsBuild,
			OSPlatform:      pbEnrollDetails.OsPlatform,
			Hostname:        pbEnrollDetails.Hostname,
			HardwareVendor:  pbEnrollDetails.HardwareVendor,
			HardwareModel:   pbEnrollDetails.HardwareModel,
			HardwareSerial:  pbEnrollDetails.HardwareSerial,
			OsqueryVersion:  pbEnrollDetails.OsqueryVersion,
			LauncherVersion: pbEnrollDetails.LauncherVersion,
			OSName:          pbEnrollDetails.OsName,
			OSPlatformLike:  pbEnrollDetails.OsPlatformLike,
		}
	}
	return enrollmentRequest{
		EnrollSecret:      req.EnrollSecret,
		HostIdentifier:    req.HostIdentifier,
		EnrollmentDetails: enrollDetails,
	}, nil
}

func decodeJSONRPCEnrollmentResponse(_ context.Context, res jsonrpc.Response) (interface{}, error) {
	if res.Error != nil {
		return nil, *res.Error
	}
	var result enrollmentResponse
	err := json.Unmarshal(res.Result, &result)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshalling RequestEnrollment response")
	}

	return result, nil
}

func encodeGRPCEnrollmentRequest(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(enrollmentRequest)
	enrollDetails := &pb.EnrollmentDetails{
		OsVersion:       req.EnrollmentDetails.OSVersion,
		OsBuild:         req.EnrollmentDetails.OSBuildID,
		OsPlatform:      req.EnrollmentDetails.OSPlatform,
		Hostname:        req.EnrollmentDetails.Hostname,
		HardwareVendor:  req.EnrollmentDetails.HardwareVendor,
		HardwareModel:   req.EnrollmentDetails.HardwareModel,
		HardwareSerial:  req.EnrollmentDetails.HardwareSerial,
		OsqueryVersion:  req.EnrollmentDetails.OsqueryVersion,
		LauncherVersion: req.EnrollmentDetails.LauncherVersion,
		OsName:          req.EnrollmentDetails.OSName,
		OsPlatformLike:  req.EnrollmentDetails.OSPlatformLike,
	}
	return &pb.EnrollmentRequest{
		EnrollSecret:      req.EnrollSecret,
		HostIdentifier:    req.HostIdentifier,
		EnrollmentDetails: enrollDetails,
	}, nil
}

func decodeGRPCEnrollmentResponse(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*pb.EnrollmentResponse)
	return enrollmentResponse{
		NodeKey:     req.NodeKey,
		NodeInvalid: req.NodeInvalid,
	}, nil
}

func encodeGRPCEnrollmentResponse(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(enrollmentResponse)
	resp := &pb.EnrollmentResponse{
		NodeKey:     req.NodeKey,
		NodeInvalid: req.NodeInvalid,
	}
	return encodeResponse(resp, req.Err)
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
	newCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	request := enrollmentRequest{EnrollSecret: enrollSecret, HostIdentifier: hostIdentifier, EnrollmentDetails: details}
	response, err := e.RequestEnrollmentEndpoint(newCtx, request)
	if err != nil {
		return "", false, err
	}
	resp := response.(enrollmentResponse)
	return resp.NodeKey, resp.NodeInvalid, resp.Err
}

func (s *grpcServer) RequestEnrollment(ctx context.Context, req *pb.EnrollmentRequest) (*pb.EnrollmentResponse, error) {
	_, rep, err := s.enrollment.ServeGRPC(ctx, req)
	if err != nil {
		return nil, err
	}
	return rep.(*pb.EnrollmentResponse), nil
}

func (mw logmw) RequestEnrollment(ctx context.Context, enrollSecret, hostIdentifier string, details EnrollmentDetails) (nodekey string, reauth bool, err error) {
	defer func(begin time.Time) {
		uuid, _ := uuid.FromContext(ctx)

		keyvals := []interface{}{
			"method", "RequestEnrollment",
			"uuid", uuid,
			"hostIdentifier", hostIdentifier,
			"reauth", reauth,
			"err", err,
			"took", time.Since(begin),
		}

		logger := level.Debug(mw.logger)
		if err != nil {
			logger = level.Info(mw.logger)
			keyvals = append(keyvals,
				"enrollSecret", enrollSecret,
				"nodekey", nodekey,
			)
		}
		logger.Log(keyvals...)

	}(time.Now())

	nodekey, reauth, err = mw.next.RequestEnrollment(ctx, enrollSecret, hostIdentifier, details)
	return nodekey, reauth, err
}

func (mw uuidmw) RequestEnrollment(ctx context.Context, enrollSecret, hostIdentifier string, details EnrollmentDetails) (errcode string, reauth bool, err error) {
	ctx = uuid.NewContext(ctx, uuid.NewForRequest())
	return mw.next.RequestEnrollment(ctx, enrollSecret, hostIdentifier, details)
}
