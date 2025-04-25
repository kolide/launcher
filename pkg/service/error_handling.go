package service

import (
	"encoding/json"

	"github.com/go-kit/kit/transport/http/jsonrpc"
)

func encodeJSONResponse(resp json.RawMessage, err error) (json.RawMessage, error) {
	if err == nil {
		return resp, nil
	}

	// Encode as jsonrpc error
	return nil, &jsonrpc.Error{
		Code:    -32000,
		Message: "Server Error",
		Data:    err,
	}
}
