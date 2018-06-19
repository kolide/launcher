package service

import (
	"context"
	"time"

	"github.com/go-kit/kit/endpoint"
	"github.com/kolide/kit/contexts/uuid"
	"github.com/pkg/errors"

	pb "github.com/kolide/launcher/service/internal/launcherproto"
)

type enrollmentRequest struct {
	EnrollSecret      string
	HostIdentifier    string
	EnrollmentDetails EnrollmentDetails
}

type EnrollmentDetails struct {
	OSVersion       string
	OSBuildID       string
	OSPlatform      string
	Hostname        string
	HardwareVendor  string
	HardwareModel   string
	HardwareSerial  string
	OsqueryVersion  string
	LauncherVersion string
}

type enrollmentResponse struct {
	NodeKey     string
	NodeInvalid bool
	Err         error
}

func decodeGRPCEnrollmentRequest(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*pb.EnrollmentRequest)
	pbEnrollDetails := req.GetEnrollmentDetails()
	enrollDetails := EnrollmentDetails{
		OSVersion:       pbEnrollDetails.OsVersion,
		OSBuildID:       pbEnrollDetails.OsBuild,
		OSPlatform:      pbEnrollDetails.OsPlatform,
		Hostname:        pbEnrollDetails.Hostname,
		HardwareVendor:  pbEnrollDetails.HardwareVendor,
		HardwareModel:   pbEnrollDetails.HardwareModel,
		HardwareSerial:  pbEnrollDetails.HardwareSerial,
		OsqueryVersion:  pbEnrollDetails.OsqueryVersion,
		LauncherVersion: pbEnrollDetails.LauncherVersion,
	}
	return enrollmentRequest{
		EnrollSecret:      req.EnrollSecret,
		HostIdentifier:    req.HostIdentifier,
		EnrollmentDetails: enrollDetails,
	}, nil
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
	return &pb.EnrollmentResponse{
		NodeKey:     req.NodeKey,
		NodeInvalid: req.NodeInvalid,
	}, nil
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
		return nil, errors.Wrap(err, "request enrollment")
	}
	return rep.(*pb.EnrollmentResponse), nil
}

func (mw logmw) RequestEnrollment(ctx context.Context, enrollSecret, hostIdentifier string, details EnrollmentDetails) (nodekey string, reauth bool, err error) {
	defer func(begin time.Time) {
		uuid, _ := uuid.FromContext(ctx)
		mw.logger.Log(
			"method", "RequestEnrollment",
			"uuid", uuid,
			"enrollSecret", enrollSecret,
			"hostIdentifier", hostIdentifier,
			"nodekey", nodekey,
			"reauth", reauth,
			"err", err,
			"took", time.Since(begin),
		)
	}(time.Now())

	nodekey, reauth, err = mw.next.RequestEnrollment(ctx, enrollSecret, hostIdentifier, details)
	return
}

func (mw uuidmw) RequestEnrollment(ctx context.Context, enrollSecret, hostIdentifier string, details EnrollmentDetails) (errcode string, reauth bool, err error) {
	ctx = uuid.NewContext(ctx, uuid.NewForRequest())
	return mw.next.RequestEnrollment(ctx, enrollSecret, hostIdentifier, details)
}
