package checkpoint

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
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

// Run starts a log checkpoint routine. The purpose of this is to
// ensure we get good debugging information in the logs.
func Run(logger logger, db *bbolt.DB, opts launcher.Options) {

	// Things to add:
	//  * invoke osquery for better hardware info
	//  * runtime stats, like memory allocations

	go func() {
		logCheckPoint(logger, db, opts)

		for range time.Tick(time.Minute * 60) {
			logCheckPoint(logger, db, opts)
		}
	}()
}

func logCheckPoint(logger log.Logger, db *bbolt.DB, opts launcher.Options) {
	logger = log.With(logger, "msg", "log checkpoint")

	logger.Log("hostname", hostName())
	logger.Log("runtime", runtimeInfo)
	logger.Log("launcher", launcherInfo)
	logger.Log("notableFiles", fileNamesInDirs(notableFileDirs...))
	logDbSize(logger, db)
	logConnections(logger, opts)
	logIpLookups(logger, opts)
	logKolideServerVersion(logger, opts)
	logNotaryVersions(logger, opts)
}

func logDbSize(logger log.Logger, db *bbolt.DB) {
	boltStats, err := agent.GetStats(db)
	if err != nil {
		logger.Log("bbolt db size", err.Error())
	} else {
		logger.Log("bbolt db size", boltStats.DB.Size)
	}
}

func logKolideServerVersion(logger logger, opts launcher.Options) {
	if !opts.KolideHosted {
		return
	}

	httpClient := &http.Client{Timeout: requestTimeout}

	kolideServerUrl, err := parseUrl(fmt.Sprintf("%s/version", opts.KolideServerURL), opts)
	if err != nil {
		logger.Log("kolide server version fetch", err)
	} else {
		logger.Log("kolide server version fetch", fetchFromUrls(httpClient, kolideServerUrl))
	}
}

func logNotaryVersions(logger logger, opts launcher.Options) {
	if !opts.KolideHosted || !opts.Autoupdate {
		return
	}

	httpClient := &http.Client{Timeout: requestTimeout}

	notaryUrl, err := parseUrl(fmt.Sprintf("%s/v2/kolide/launcher/_trust/tuf/targets/releases.json", opts.NotaryServerURL), opts)
	if err != nil {
		logger.Log("notary versions", err)
	} else {
		logger.Log("notary versions", fetchNotaryVersions(httpClient, notaryUrl))
	}
}

func logConnections(logger logger, opts launcher.Options) {
	dialer := &net.Dialer{Timeout: requestTimeout}
	logger.Log("connections", testConnections(dialer, urlsToTest(opts)...))
}

func logIpLookups(logger logger, opts launcher.Options) {
	ipLookuper := &net.Resolver{}
	logger.Log("ip loook ups", lookupHostsIpv4s(ipLookuper, urlsToTest(opts)...))
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
