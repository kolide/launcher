package history

import (
	"fmt"

	"github.com/pkg/errors"
)

type Instance struct {
	StartTime   string
	ConnectTime string
	ExitTime    string
	Hostname    string
	InstanceId  string
	Version     string
	Error       error
}

type Querier interface {
	Query(query string) ([]map[string]string, error)
}

type ExpectedAtLeastOneRowError struct{}

func (e ExpectedAtLeastOneRowError) Error() string {
	return "expected at least one row from osquery_info table"
}

func (i *Instance) connected(querier Querier) error {
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

	return nil
}

func (i *Instance) exited(err error) {
	if err != nil {
		i.addError(err)
	}

	i.ExitTime = timeNow()
}

func (i *Instance) addError(err error) {
	if i.Error != nil {
		i.Error = fmt.Errorf("%v: %v", i.Error, err)
		return
	}

	i.Error = err
}
