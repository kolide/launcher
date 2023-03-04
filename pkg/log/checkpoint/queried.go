package checkpoint

import (
	"errors"
	"fmt"
)

const osSqlQuery = `
SELECT
	os_version.build as os_build,
	os_version.name as os_name,
	os_version.platform as os_platform,
	os_version.platform_like as os_platform_like,
	os_version.version as os_version
FROM
	os_version
`

const systemSqlQuery = `
SELECT
	system_info.hardware_model,
	system_info.hardware_serial,
	system_info.hardware_vendor,
	system_info.hostname,
	system_info.uuid as hardware_uuid
FROM
	system_info
`

const osquerySqlQuery = `
SELECT
	osquery_info.version as osquery_version,
	osquery_info.instance_id as osquery_instance_id
FROM
    osquery_info
`

func (c *checkPointer) logOsqueryInfo() {
	if c.querier == nil {
		return
	}

	info, err := c.query(osquerySqlQuery)
	if err != nil {
		c.logger.Log("msg", "error querying osquery info", "err", err)
		return
	}

	c.logger.Log("osquery_info", info)
}

// queryStaticInfo usually the querier to add additional static info.
func (c *checkPointer) queryStaticInfo() {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.querier == nil || c.staticQueried {
		return
	}

	if info, err := c.query(osSqlQuery); err != nil {
		c.logger.Log("msg", "failed to query os info", "err", err)
		return
	} else {
		c.queriedInfo["os_info"] = info
	}

	if info, err := c.query(systemSqlQuery); err != nil {
		c.logger.Log("msg", "failed to query os info", "err", err)
		return
	} else {
		c.queriedInfo["system_info"] = info
	}

	c.staticQueried = true
}

func (c *checkPointer) query(sql string) (map[string]string, error) {
	if c.querier == nil {
		return nil, errors.New("no querier")
	}

	resp, err := c.querier.Query(sql)
	if err != nil {
		return nil, fmt.Errorf("error querying for static: %s", err)
	}

	if len(resp) < 1 {
		return nil, errors.New("expected at least one row for static details")
	}

	return resp[0], nil
}
