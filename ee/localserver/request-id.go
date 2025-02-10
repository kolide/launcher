package localserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/traces"
)

type (
	identifiers struct {
		UUID           string
		InstanceId     string
		HardwareSerial string
	}

	requestIdsResponse struct {
		RequestId string
		identifiers
		Nonce             string
		Timestamp         time.Time
		Status            status
		Origin            string
		EnrollmentDetails types.EnrollmentDetails
	}

	status struct {
		EnrollmentStatus string
		InstanceStatuses map[string]types.InstanceStatus
	}
)

const (
	idSQL = "select instance_id, osquery_info.uuid, hardware_serial from osquery_info, system_info"
)

func (ls *localServer) updateIdFields() error {
	if ls.querier == nil {
		return errors.New("no querier set")
	}

	results, err := ls.querier.Query(idSQL)
	if err != nil {
		return fmt.Errorf("id query failed: %w", err)
	}

	if results == nil || len(results) < 1 {
		return errors.New("id query didn't return data")
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

func (ls *localServer) requestIdHandlerFunc(w http.ResponseWriter, r *http.Request) {
	r, span := traces.StartHttpRequestSpan(r, "path", r.URL.Path)
	defer span.End()

	enrollmentStatus, _ := ls.knapsack.CurrentEnrollmentStatus()
	enrollmentDetails := ls.knapsack.GetEnrollmentDetails()

	response := requestIdsResponse{
		Nonce:     ulid.New(),
		Timestamp: time.Now(),
		Origin:    r.Header.Get("Origin"),
		Status: status{
			EnrollmentStatus: string(enrollmentStatus),
		},
		EnrollmentDetails: enrollmentDetails,
	}
	response.identifiers = ls.identifiers

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		traces.SetError(span, err)
		ls.slogger.Log(r.Context(), slog.LevelError,
			"marshaling json",
			"err", err,
		)

		jsonBytes = []byte(fmt.Sprintf("unable to marshal json: %v", err))
	}

	w.Write(jsonBytes)
}
