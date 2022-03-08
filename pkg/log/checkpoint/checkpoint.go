package checkpoint

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"go.etcd.io/bbolt"
)

// if we find any files in these directories, log them to check point
var notableFileDirs = []string{"/var/osquery", "/etc/osquery"}

// log these hosts' IPs to check point
var hostsToCheckConnectivity = []string{"k2device.kolide.com", "k2control.kolide.com", "notary.kolide.co", "dl.kolide.co"}

// log get data from these urls
var fetchUrls = []string{"https://k2device.kolide.com/version"}

// logger is an interface that allows mocking of logger
type logger interface {
	Log(keyvals ...interface{}) error
}

// Run starts a log checkpoint routine. The purpose of this is to
// ensure we get good debugging information in the logs.
func Run(logger logger, db *bbolt.DB) {

	// Things to add:
	//  * database sizes
	//  * server ping ... see what IP the urls resolve to
	//  * invoke osquery for better hardware info
	//  * runtime stats, like memory allocations

	go func() {
		logCheckPoint(logger)

		for range time.Tick(time.Minute * 60) {
			logCheckPoint(logger)
		}
	}()
}

func logCheckPoint(logger log.Logger) {
	logger.Log(
		"msg", "log checkpoint started",
		"hostname", hostName(),
		"notableFiles", fileNamesInDirs(notableFileDirs...),
		"IPs", lookupHostsIpv4s(net.DefaultResolver, hostsToCheckConnectivity...),
		"connectivity", testConnections(&net.Dialer{Timeout: 5 * time.Second}, hostsToCheckConnectivity...),
		"fetches", fetchFromUrls(&http.Client{Timeout: 5 * time.Second}, fetchUrls),
		"fetch-notary-version", fetchNotaryVersion(&http.Client{Timeout: 5 * time.Second}, "https://notary.kolide.com/v2/kolide/launcher/_trust/tuf/targets/releases.json"),
	)
}

func hostName() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = fmt.Sprintf("ERROR: %s", err)
	}

	return hostname
}
