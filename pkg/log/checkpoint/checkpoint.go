package checkpoint

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/launcher"
	"go.etcd.io/bbolt"
)

// defines time out for all http, dns, connectivity requests
const requestTimeout = time.Second * 5

var (
	// if we find any files in these directories, log them to check point
	notableFileDirs = []string{"/var/osquery", "/etc/osquery"}

	runtimeInfo = map[string]string{
		"GOARCH": runtime.GOARCH,
		"GOOS":   runtime.GOOS,
	}

	launcherInfo = map[string]string{
		"revision": version.Version().Revision,
		"version":  version.Version().Version,
	}
)

// logger is an interface that allows mocking of logger
type logger interface {
	Log(keyvals ...interface{}) error
}

type querierInt interface {
	Query(query string) ([]map[string]string, error)
}

type checkPointer struct {
	logger  logger
	querier querierInt
	db      *bbolt.DB
	opts    launcher.Options

	lock          sync.RWMutex
	queriedInfo   map[string]any
	staticQueried bool
}

func New(logger logger, db *bbolt.DB, opts launcher.Options) *checkPointer {
	return &checkPointer{
		logger: log.With(logger, "msg", "log checkpoint"),
		db:     db,
		opts:   opts,

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
func (c *checkPointer) Run() {
	go func() {
		c.logCheckPoint()

		for range time.Tick(time.Minute * 60) {
			c.logCheckPoint()
		}
	}()
}

func (c *checkPointer) logCheckPoint() {
	// populate and log the queried static info
	c.queryStaticInfo()
	c.logQueriedInfo()

	c.logger.Log("runtime", runtimeInfo)
	c.logger.Log("launcher", launcherInfo)
	c.logger.Log("hostname", hostName())
	c.logger.Log("notableFiles", fileNamesInDirs(notableFileDirs...))

	c.logOsqueryInfo()
	c.logDbSize()
	c.logKolideServerVersion()
	c.logConnections()
	c.logIpLookups()
	c.logNotaryVersions()
}

func (c *checkPointer) logQueriedInfo() {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if !c.staticQueried {
		return
	}

	for k, v := range c.queriedInfo {
		c.logger.Log(k, v)
	}
}

func (c *checkPointer) logDbSize() {
	boltStats, err := agent.GetStats(c.db)
	if err != nil {
		c.logger.Log("bbolt db size", err.Error())
	} else {
		c.logger.Log("bbolt db size", boltStats.DB.Size)
	}
}

func (c *checkPointer) logKolideServerVersion() {
	if !c.opts.KolideHosted {
		return
	}

	httpClient := &http.Client{Timeout: requestTimeout}

	kolideServerUrl, err := parseUrl(fmt.Sprintf("%s/version", c.opts.KolideServerURL), c.opts)
	if err != nil {
		c.logger.Log("kolide server version fetch", err)
	} else {
		c.logger.Log("kolide server version fetch", fetchFromUrls(httpClient, kolideServerUrl))
	}
}

func (c *checkPointer) logNotaryVersions() {
	if !c.opts.KolideHosted || !c.opts.Autoupdate {
		return
	}

	httpClient := &http.Client{Timeout: requestTimeout}

	notaryUrl, err := parseUrl(fmt.Sprintf("%s/v2/kolide/launcher/_trust/tuf/targets/releases.json", c.opts.NotaryServerURL), c.opts)
	if err != nil {
		c.logger.Log("notary versions", err)
	} else {
		c.logger.Log("notary versions", fetchNotaryVersions(httpClient, notaryUrl))
	}
}

func (c *checkPointer) logConnections() {
	dialer := &net.Dialer{Timeout: requestTimeout}
	c.logger.Log("connections", testConnections(dialer, urlsToTest(c.opts)...))
}

func (c *checkPointer) logIpLookups() {
	ipLookuper := &net.Resolver{}
	c.logger.Log("ip look ups", lookupHostsIpv4s(ipLookuper, urlsToTest(c.opts)...))
}

func urlsToTest(opts launcher.Options) []*url.URL {
	addrsToTest := []string{opts.KolideServerURL}

	if opts.Autoupdate {
		addrsToTest = append(addrsToTest, opts.MirrorServerURL, opts.NotaryServerURL)
	}

	if opts.Control {
		addrsToTest = append(addrsToTest, opts.ControlServerURL)
	}

	urls := []*url.URL{}

	for _, addr := range addrsToTest {

		url, urlErr := parseUrl(addr, opts)

		if urlErr != nil {
			continue
		}

		urls = append(urls, url)
	}

	return urls
}

func parseUrl(addr string, opts launcher.Options) (*url.URL, error) {

	if !strings.HasPrefix(addr, "http") {
		scheme := "https"
		if opts.InsecureTransport {
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
		if opts.InsecureTransport {
			port = "80"
		}
		u.Host = net.JoinHostPort(u.Host, port)
	}

	return u, nil
}

func hostName() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = fmt.Sprintf("ERROR: %s", err)
	}

	return hostname
}
