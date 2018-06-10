package service

import (
	"context"
	"time"

	"github.com/go-kit/kit/endpoint"
	pb "github.com/kolide/launcher/service/internal/launcherproto"
	"github.com/pkg/errors"
)

type enrollmentRequest struct {
	EnrollSecret   string
	HostIdentifier string
}

type enrollmentResponse struct {
	NodeKey     string
	NodeInvalid bool
	Err         error
}

func decodeGRPCEnrollmentRequest(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*pb.EnrollmentRequest)
	return enrollmentRequest{
		EnrollSecret:   req.EnrollSecret,
		HostIdentifier: req.HostIdentifier,
	}, nil
}

func encodeGRPCEnrollmentRequest(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(enrollmentRequest)
	return &pb.EnrollmentRequest{
		EnrollSecret:   req.EnrollSecret,
		HostIdentifier: req.HostIdentifier,
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
		nodeKey, valid, err := svc.RequestEnrollment(ctx, req.EnrollSecret, req.HostIdentifier)
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
func (e Endpoints) RequestEnrollment(ctx context.Context, enrollSecret, hostIdentifier string) (string, bool, error) {
	newCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	request := enrollmentRequest{EnrollSecret: enrollSecret, HostIdentifier: hostIdentifier}
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
