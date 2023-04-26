package checkpoint

import (
	"crypto/x509"
	"encoding/base64"
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
	"github.com/kolide/launcher/pkg/agent/types"
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
	logger   logger
	knapsack types.Knapsack
	querier  querierInt

	lock        sync.RWMutex
	queriedInfo map[string]any
}

func New(logger logger, k types.Knapsack) *checkPointer {
	return &checkPointer{
		logger:   log.With(logger, "component", "log checkpoint"),
		knapsack: k,

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
	c.logger.Log("notableFiles", filesInDirs(notableFileDirs...))
	c.logger.Log("keyinfo", agentKeyInfo())
	c.logOsqueryInfo()
	c.logDbSize()
	c.logKolideServerVersion()
	c.logConnections()
	c.logIpLookups()
	c.logNotaryVersions()
	c.logServerProvidedData()
}

func (c *checkPointer) logDbSize() {
	boltStats, err := agent.GetStats(c.knapsack.BboltDB())
	if err != nil {
		c.logger.Log("bbolt db size", err.Error())
	} else {
		c.logger.Log("bbolt db size", boltStats.DB.Size)
	}
}

func (c *checkPointer) logKolideServerVersion() {
	if !c.knapsack.KolideHosted() {
		return
	}

	httpClient := &http.Client{Timeout: requestTimeout}

	kolideServerUrl, err := parseUrl(fmt.Sprintf("%s/version", c.knapsack.KolideServerURL()), c.knapsack)
	if err != nil {
		c.logger.Log("kolide server version fetch", err)
	} else {
		c.logger.Log("kolide server version fetch", fetchFromUrls(httpClient, kolideServerUrl))
	}
}

func (c *checkPointer) logNotaryVersions() {
	if !c.knapsack.KolideHosted() || !c.knapsack.Autoupdate() {
		return
	}

	httpClient := &http.Client{Timeout: requestTimeout}

	notaryUrl, err := parseUrl(fmt.Sprintf("%s/v2/kolide/launcher/_trust/tuf/targets/releases.json", c.knapsack.NotaryServerURL()), c.knapsack)
	if err != nil {
		c.logger.Log("notary versions", err)
	} else {
		c.logger.Log("notary versions", fetchNotaryVersions(httpClient, notaryUrl))
	}
}

func (c *checkPointer) logConnections() {
	dialer := &net.Dialer{Timeout: requestTimeout}
	c.logger.Log("connections", testConnections(dialer, urlsToTest(c.knapsack)...))
}

func (c *checkPointer) logIpLookups() {
	ipLookuper := &net.Resolver{}
	c.logger.Log("ip look ups", lookupHostsIpv4s(ipLookuper, urlsToTest(c.knapsack)...))
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

func hostName() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = fmt.Sprintf("ERROR: %s", err)
	}

	return hostname
}

func agentKeyInfo() map[string]string {
	keyinfo := make(map[string]string, 3)

	pub := agent.LocalDbKeys().Public()
	if pub == nil {
		keyinfo["local_key"] = "nil. Likely startup delay"
		return keyinfo
	}

	if localKeyDer, err := x509.MarshalPKIXPublicKey(pub); err == nil {
		// der is a binary format, so convert to b64
		keyinfo["local_key"] = base64.StdEncoding.EncodeToString(localKeyDer)
	} else {
		keyinfo["local_key"] = fmt.Sprintf("error marshalling local key (startup is sometimes weird): %s", err)
	}

	// We don't always have hardware keys. Move on if we don't
	if agent.HardwareKeys().Public() == nil {
		return keyinfo
	}

	if hardwareKeyDer, err := x509.MarshalPKIXPublicKey(agent.HardwareKeys().Public()); err == nil {
		// der is a binary format, so convert to b64
		keyinfo["hardware_key"] = base64.StdEncoding.EncodeToString(hardwareKeyDer)
		keyinfo["hardware_key_source"] = agent.HardwareKeys().Type()
	}

	return keyinfo
}
