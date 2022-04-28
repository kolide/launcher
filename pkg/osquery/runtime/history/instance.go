package history

import (
	"github.com/pkg/errors"
)

type Instance struct {
	StartTime   string
	ConnectTime string
	ExitTime    string
	Hostname    string
	InstanceId  string
	Version     string
	Error       string
}

type Querier interface {
	Query(query string) ([]map[string]string, error)
}

type ExpectedAtLeastOneRowError struct{}

func (e ExpectedAtLeastOneRowError) Error() string {
	return "expected at least one row from osquery_info table"
}

// Connected sets the connect time and instance id of the current osquery instance
func (i *Instance) Connected(querier Querier) error {
	currentHistory.Lock()
	defer currentHistory.Unlock()

	results, err := querier.Query("select instance_id, version from osquery_info order by start_time limit 1")
	if err != nil {
		return err
	}

	if results == nil || len(results) < 1 {
		return ExpectedAtLeastOneRowError{}
	}

	instanceId, ok := results[0]["instance_id"]
	if !ok {
		return errors.New("instance_id column did not type check to string")
	}

	version, ok := results[0]["version"]
	if !ok {
		return errors.New("version column did not type check to string")
	}

	i.ConnectTime = timeNow()
	i.InstanceId = instanceId
	i.Version = version

	if err := currentHistory.save(); err != nil {
		return errors.Wrap(err, "error saving osquery_instance_history")
	}

	return nil
}

// InstanceExited sets the exit time and appends provided error (if any) to current osquery instance
func (i *Instance) Exited(exitError error) error {
	currentHistory.Lock()
	defer currentHistory.Unlock()

	if exitError != nil {
		i.Error = exitError.Error()
	}

	i.ExitTime = timeNow()

	err := currentHistory.save()

	if err != nil {
		return errors.Wrap(err, "error saving osquery_instance_history")
	}

	return nil
}
