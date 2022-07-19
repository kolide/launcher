package localserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/ulid"
)

type requestIdRequest struct {
	RequestID string
}

type identifiers struct {
	UUID           string
	InstanceId     string
	HardwareSerial string
}

type requestIdsResponse struct {
	RequestId string
	identifiers
	Nonce     string
	Timestamp time.Time
}

type Querier interface {
	Query(query string) ([]map[string]string, error)
}

type ExpectedAtLeastOneRowError struct{}

func (e ExpectedAtLeastOneRowError) Error() string {
	return "expected at least one row from osquery_info table"
}

func (ls *localServer) UpdateIdFields(querier Querier) error {
	results, err := querier.Query("select instance_id, osquery_info.uuid, hardware_serial from osquery_info, system_info")
	if err != nil {
		return err
	}

	if results == nil || len(results) < 1 {
		return ExpectedAtLeastOneRowError{}
	}

	if id, ok := results[0]["instance_id"]; ok {
		ls.identifiers.InstanceId = id
	}

	if uuid, ok := results[0]["uuid"]; ok {
		ls.identifiers.UUID = uuid
	}
	if hs, ok := results[0]["hardware_serial"]; ok {
		ls.identifiers.HardwareSerial = hs
	}

	return nil
}

func (ls *localServer) requestIdHandler() http.Handler {
	return http.HandlerFunc(ls.requestIdHandlerFunc)
}

func (ls *localServer) requestIdHandlerFunc(res http.ResponseWriter, req *http.Request) {
	response := requestIdsResponse{
		RequestId: ulid.New(), //FIXME
		Nonce:     ulid.New(),
		Timestamp: time.Now(),
	}
	response.identifiers = ls.identifiers

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		level.Info(ls.logger).Log("msg", "unable to marshal json", "err", err)
		jsonBytes = []byte(fmt.Sprintf("unable to marshal json: %w", err))
	}

	res.Write(jsonBytes)
}
