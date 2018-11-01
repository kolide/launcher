package service

import (
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type osqueryError interface {
	error
	NodeInvalid() bool
}

func isNodeInvalidErr(err error) bool {
	oe, ok := errors.Cause(err).(osqueryError)
	return ok && oe.NodeInvalid()
}

func encodeResponse(resp interface{}, err error) (interface{}, error) {
	switch {
	case err == nil:
		return resp, nil
	case isNodeInvalidErr(err):
		return nil, status.Error(codes.Unauthenticated, "Node Invalid")
	default:
		return nil, status.Error(codes.Unknown, "Server Error")
	}
}
