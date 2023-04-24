package localserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/user"
	"runtime"
	"time"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/consoleuser"
	"github.com/kolide/launcher/pkg/backoff"
)

type identifiers struct {
	UUID           string
	InstanceId     string
	HardwareSerial string
}

type requestIdsResponse struct {
	RequestId string
	identifiers
	Nonce        string
	Timestamp    time.Time
	ConsoleUsers []*user.User
}

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

func (ls *localServer) requestIdHandlerFunc(res http.ResponseWriter, req *http.Request) {
	response := requestIdsResponse{
		Nonce:     ulid.New(),
		Timestamp: time.Now(),
	}
	response.identifiers = ls.identifiers

	consoleUsers, err := consoleUsers()
	if err != nil {
		level.Error(ls.logger).Log(
			"msg", "getting console users",
			"err", err,
		)
		response.ConsoleUsers = []*user.User{}
	} else {
		response.ConsoleUsers = consoleUsers
	}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		level.Info(ls.logger).Log("msg", "unable to marshal json", "err", err)
		jsonBytes = []byte(fmt.Sprintf("unable to marshal json: %v", err))
	}

	res.Write(jsonBytes)
}

func consoleUsers() ([]*user.User, error) {
	const maxDuration = 1 * time.Second
	context, cancel := context.WithTimeout(context.Background(), maxDuration)
	defer cancel()

	var users []*user.User

	return users, backoff.WaitFor(func() error {
		uids, err := consoleuser.CurrentUids(context)
		if err != nil {
			return err
		}

		for _, uid := range uids {
			var err error
			var u *user.User
			if runtime.GOOS == "windows" {
				u, err = user.Lookup(uid)
			} else {
				u, err = user.LookupId(uid)
			}
			if err != nil {
				return err
			}

			users = append(users, u)
		}
		return nil
	}, maxDuration, 250*time.Millisecond)
}
