package localserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/pkg/osquery/runsimple"
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
	idSQL = "select instance_id, osquery_info.uuid, hardware_serial from osquery_info, system_info;"
)

func (ls *localServer) updateIdFields() error {
	ctx, span := observability.StartSpan(context.TODO())
	defer span.End()

	osquerydPath := ls.knapsack.LatestOsquerydPath(ctx)

	var respBytes bytes.Buffer
	var stderrBytes bytes.Buffer

	osq, err := runsimple.NewOsqueryProcess(
		osquerydPath,
		runsimple.WithStdout(&respBytes),
		runsimple.WithStderr(&stderrBytes),
	)
	if err != nil {
		return fmt.Errorf("creating osquery runsimple process to query localserver ID fields: %w", err)
	}

	// This is running in the background, so we can afford a longer timeout here
	osqCtx, osqCancel := context.WithTimeout(ctx, 15*time.Second)
	defer osqCancel()

	if sqlErr := osq.RunSql(osqCtx, []byte(idSQL)); osqCtx.Err() != nil {
		return fmt.Errorf("querying for localserver ID fields: context error: %w: stderr: %s", osqCtx.Err(), stderrBytes.String())
	} else if sqlErr != nil {
		return fmt.Errorf("querying for localserver ID fields: %w; stderr: %s", sqlErr, stderrBytes.String())
	}

	var results []map[string]string
	if err := json.Unmarshal(respBytes.Bytes(), &results); err != nil {
		return fmt.Errorf("unmarshalling localserver ID fields response: %w; stderr: %s", err, stderrBytes.String())
	}

	if len(results) < 1 {
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
	r, span := observability.StartHttpRequestSpan(r, "path", r.URL.Path)
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
		observability.SetError(span, err)
		ls.slogger.Log(r.Context(), slog.LevelError,
			"marshaling json",
			"err", err,
		)

		jsonBytes = []byte(fmt.Sprintf("unable to marshal json: %v", err))
	}

	w.Write(jsonBytes)
}
