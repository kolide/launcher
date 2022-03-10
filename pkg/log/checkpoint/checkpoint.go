package checkpoint

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

// defines time out for all http, dns, connectivity requests
const requestTimeout = time.Second * 5

// if we find any files in these directories, log them to check point
var notableFileDirs = []string{"/var/osquery", "/etc/osquery"}

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

	boltStats, err := agent.GetStats(db)
	if err != nil {
		logger.Log("bbolt db size", err.Error())
	} else {
		logger.Log("bbolt db size", boltStats.DB.Size)
	}

	logger.Log("hostname", hostName())
	logger.Log("notableFiles", fileNamesInDirs(notableFileDirs...))

	logConnections(logger, opts)
	logIpLookups(logger, opts)
	logKolideServerVersion(logger, opts)
	logNotaryVersions(logger, opts)
}

func logKolideServerVersion(logger logger, opts launcher.Options) {
	if !opts.KolideHosted {
		return
	}

	httpClient := &http.Client{Timeout: requestTimeout}

	kolideServerUrl, err := url.Parse(fmt.Sprintf("https://%s/version", opts.KolideServerURL))
	if err != nil {
		logger.Log("url parse error", err)
	} else {
		logger.Log("kolide server version fetch", fetchFromUrls(httpClient, kolideServerUrl))
	}
}

func logNotaryVersions(logger logger, opts launcher.Options) {
	if !opts.KolideHosted || !opts.Autoupdate {
		return
	}

	httpClient := &http.Client{Timeout: requestTimeout}

	notaryUrl, err := url.Parse(fmt.Sprintf("https://%s/v2/kolide/launcher/_trust/tuf/targets/releases.json", opts.NotaryServerURL))
	if err != nil {
		logger.Log("url parse error", err)
	} else {
		logger.Log("notary versions", fetchNotaryVersions(httpClient, notaryUrl))
	}
}

func logConnections(logger logger, opts launcher.Options) {
	urls, err := urlsToTest(opts)

	if err != nil {
		logger.Log("url parse errors", err)
	}

	dialer := &net.Dialer{Timeout: requestTimeout}
	logger.Log("connections", testConnections(dialer, urls...))
}

func logIpLookups(logger logger, opts launcher.Options) {
	urls, err := urlsToTest(opts)

	if err != nil {
		logger.Log("url parse errors", err)
	}

	ipLookuper := &net.Resolver{}
	logger.Log("ip loook ups", lookupHostsIpv4s(ipLookuper, urls...))
}

func urlsToTest(opts launcher.Options) ([]*url.URL, error) {
	addrsToTest := []string{opts.KolideServerURL}

	if opts.Autoupdate {
		addrsToTest = append(addrsToTest, opts.MirrorServerURL, opts.NotaryServerURL)
	}

	if opts.Control {
		addrsToTest = append(addrsToTest, opts.ControlServerURL)
	}

	urls := []*url.URL{}
	var err error

	for _, addr := range addrsToTest {
		url, urlErr := url.Parse(fmt.Sprintf("https://%s", addr))

		switch {
		// first error
		case urlErr != nil && err == nil:
			err = urlErr

		// not first error
		case urlErr != nil && err != nil:
			err = errors.Wrap(err, urlErr.Error())

		// no error
		default:
			urls = append(urls, url)
		}
	}

	return urls, err
}

func hostName() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = fmt.Sprintf("ERROR: %s", err)
	}

	return hostname
}
