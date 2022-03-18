package osquery_instance_history

import (
	"fmt"

	"github.com/pkg/errors"
)

type Instance struct {
	StartTime   string
	ConnectTime string
	ExitTime    string
	InstanceId  string
	Error       error
}

type Querier interface {
	Query(query string) ([]map[string]string, error)
}

func (i *Instance) connected(querier Querier) error {
	results, err := querier.Query("select instance_id from osquery_info order by start_time limit 1")
	if err != nil {
		return err
	}

	if len(results) < 1 {
		return errors.New("expected at least one row from osquery_info table")
	}

	if val, ok := results[0]["instance_id"]; ok {
		i.ConnectTime = timeNow()
		i.InstanceId = val
		return nil
	}

	return errors.New("instance_id column did not type check to string")
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
