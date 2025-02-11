package history

import (
	"errors"

	"github.com/kolide/launcher/ee/agent/types"
)

type Instance struct {
	RegistrationId string // which registration this instance belongs to
	RunId          string // ID for instance, assigned by launcher
	StartTime      string
	ConnectTime    string
	ExitTime       string
	Hostname       string
	InstanceId     string // ID from osquery
	Version        string
	Error          string
}

type ExpectedAtLeastOneRowError struct{}

func (e ExpectedAtLeastOneRowError) Error() string {
	return "expected at least one row from osquery_info table"
}

// Connected sets the connect time and instance id of the current osquery instance
func (i *Instance) Connected(querier types.Querier) error {
	results, err := querier.Query("select instance_id, version from osquery_info order by start_time limit 1")
	if err != nil {
		return err
	}

	if results == nil || len(results) < 1 {
		return ExpectedAtLeastOneRowError{}
	}

	instanceId, ok := results[0]["instance_id"]
	if !ok {
		return errors.New("instance_id column was not present in query results")
	}

	version, ok := results[0]["version"]
	if !ok {
		return errors.New("version column was not present in query results")
	}

	i.ConnectTime = timeNow()
	i.InstanceId = instanceId
	i.Version = version

	return nil
}

// InstanceExited sets the exit time and appends provided error (if any) to current osquery instance
func (i *Instance) Exited(exitError error) error {
	if exitError != nil {
		i.Error = exitError.Error()
	}

	i.ExitTime = timeNow()

	return nil
}
