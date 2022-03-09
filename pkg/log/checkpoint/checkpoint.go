package checkpoint

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
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
	//  * database sizes
	//  * server ping ... see what IP the urls resolve to
	//  * invoke osquery for better hardware info
	//  * runtime stats, like memory allocations

	go func() {
		logCheckPoint(logger, opts)

		for range time.Tick(time.Minute * 60) {
			logCheckPoint(logger, opts)
		}
	}()
}

func logCheckPoint(logger log.Logger, opts launcher.Options) {

	logger.Log("msg", "log checkpoint started")
	logger.Log("hostname", hostName())
	logger.Log("notableFiles", fileNamesInDirs(notableFileDirs...))

	dialer := &net.Dialer{Timeout: requestTimeout}
	ipLookuper := &net.Resolver{}
	httpClient := &http.Client{Timeout: requestTimeout}

	kolideServerUrl, err := parseUrl(opts.KolideServerURL)
	if err != nil {
		logCheckPointError(logger, errors.Wrap(err, "parsing URL"))
	}

	logMap(logger, testConnections(dialer, kolideServerUrl.Host))
	logMap(logger, lookupHostsIpv4s(ipLookuper, kolideServerUrl.Host))

	if opts.KolideHosted {
		logMap(logger, fetchFromUrls(httpClient, kolideServerUrl.String()+"/version"))
		logMap(logger, testConnections(dialer, "dl.kolide.co"))
		logMap(logger, lookupHostsIpv4s(ipLookuper, "dl.kolide.co"))
	}

	if opts.Autoupdate {
		notaryServerURL, err := parseUrl(opts.NotaryServerURL)
		if err != nil {
			logCheckPointError(logger, errors.Wrap(err, "parsing URL"))
		}

		logMap(logger, testConnections(dialer, notaryServerURL.Host))
		logMap(logger, lookupHostsIpv4s(ipLookuper, notaryServerURL.Host))

		if opts.KolideHosted {
			logMap(logger, fetchNotaryVersion(httpClient, notaryServerURL.String()+"/v2/kolide/launcher/_trust/tuf/targets/releases.json"))
		}
	}

	if opts.Autoupdate {
		notaryMirrorServerURL, err := parseUrl(opts.MirrorServerURL)
		if err != nil {
			logCheckPointError(logger, errors.Wrap(err, "parsing URL"))
		}

		logMap(logger, testConnections(dialer, notaryMirrorServerURL.Host))
		logMap(logger, lookupHostsIpv4s(ipLookuper, notaryMirrorServerURL.Host))

		if opts.KolideHosted {
			logMap(logger, fetchNotaryVersion(httpClient, notaryMirrorServerURL.String()+"/v2/kolide/launcher/_trust/tuf/targets/releases.json"))
		}
	}

	if opts.Control {
		controlServerUrl, err := parseUrl(opts.ControlServerURL)
		if err != nil {
			logCheckPointError(logger, errors.Wrap(err, "parsing URL"))
		}

		logMap(logger, testConnections(dialer, controlServerUrl.Host))
		logMap(logger, lookupHostsIpv4s(ipLookuper, controlServerUrl.Host))
	}
}

func hostName() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = fmt.Sprintf("ERROR: %s", err)
	}

	return hostname
}

func logCheckPointError(logger log.Logger, err error) {
	logger.Log("checkpoint error", err)
}

func parseUrl(addr string) (*url.URL, error) {
	return url.Parse(fmt.Sprintf("https://%s", strings.Split(addr, ":")[0]))
}

func logMap(logger log.Logger, entry map[string]interface{}) {
	for k, v := range entry {
		logger.Log(k, v)
	}
}
