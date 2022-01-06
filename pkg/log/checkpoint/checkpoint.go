package checkpoint

import (
	"fmt"
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"go.etcd.io/bbolt"
)

// Run starts a log checkpoint routine. The purpose of this is to
// ensure we get good debugging information in the logs.
func Run(logger log.Logger, db *bbolt.DB) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = fmt.Sprintf("ERROR: %s", err)
	}

	go func() {
		for range time.Tick(time.Minute * 15) {
			logger.Log(
				"msg", "checkpoint",
				"hostname", hostname,
			)
		}
	}()
}
