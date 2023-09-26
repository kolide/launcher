package checkpoint

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent/types"
)

// logger is an interface that allows mocking of logger
type logger interface {
	Log(keyvals ...interface{}) error
}

type querierInt interface {
	Query(query string) ([]map[string]string, error)
}

type checkPointer struct {
	logger    logger
	knapsack  types.Knapsack
	querier   querierInt
	interrupt chan struct{}

	lock        sync.RWMutex
	queriedInfo map[string]any
}

func New(logger logger, k types.Knapsack) *checkPointer {
	return &checkPointer{
		logger:    log.With(logger, "component", "log checkpoint"),
		knapsack:  k,
		interrupt: make(chan struct{}, 1),

		lock:        sync.RWMutex{},
		queriedInfo: make(map[string]any),
	}
}

// SetQuerier adds the querier into the checkpointer. It is done in a function, so it can happen
// later in the startup sequencing.
func (c *checkPointer) SetQuerier(querier querierInt) {
	c.querier = querier
	c.queryStaticInfo()
	c.logQueriedInfo()
}

// Run starts a log checkpoint routine. The purpose of this is to
// ensure we get good debugging information in the logs.
func (c *checkPointer) Run() error {
	ticker := time.NewTicker(time.Minute * 60)
	defer ticker.Stop()

	for {
		c.Once()

		select {
		case <-ticker.C:
			continue
		case <-c.interrupt:
			level.Debug(c.logger).Log("msg", "interrupt received, exiting execute loop")
			return nil
		}
	}
}

func (c *checkPointer) Interrupt(_ error) {
	c.interrupt <- struct{}{}
}

func (c *checkPointer) Once() {
	// populate and log the queried static info
	c.queryStaticInfo()
	c.logQueriedInfo()
	c.logOsqueryInfo()
}

func urlsToTest(flags types.Flags) []*url.URL {
	addrsToTest := []string{flags.KolideServerURL()}

	if flags.Autoupdate() {
		addrsToTest = append(addrsToTest, flags.MirrorServerURL(), flags.NotaryServerURL(), flags.TufServerURL())
	}

	if flags.ControlServerURL() != "" {
		addrsToTest = append(addrsToTest, flags.ControlServerURL())
	}

	urls := []*url.URL{}

	for _, addr := range addrsToTest {

		url, urlErr := parseUrl(addr, flags)

		if urlErr != nil {
			continue
		}

		urls = append(urls, url)
	}

	return urls
}

func parseUrl(addr string, flags types.Flags) (*url.URL, error) {
	if !strings.HasPrefix(addr, "http") {
		scheme := "https"
		if flags.InsecureTransportTLS() {
			scheme = "http"
		}
		addr = fmt.Sprintf("%s://%s", scheme, addr)
	}

	u, err := url.Parse(addr)

	if err != nil {
		return nil, err
	}

	if u.Port() == "" {
		port := "443"
		if flags.InsecureTransportTLS() {
			port = "80"
		}
		u.Host = net.JoinHostPort(u.Host, port)
	}

	return u, nil
}

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

func (c *checkPointer) logQueriedInfo() {
	c.lock.RLock()
	defer c.lock.RUnlock()

	for k, v := range c.queriedInfo {
		c.logger.Log(k, fmt.Sprintf("%+v", v))
	}
}

// queryStaticInfo usually the querier to add additional static info.
func (c *checkPointer) queryStaticInfo() {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.querier == nil {
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
