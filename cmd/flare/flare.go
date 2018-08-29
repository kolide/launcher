package main

import (
	"context"
	"crypto/x509"
	"flag"
	"net/http"
	"os"
	"time"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	grpcext "github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/service"
	"github.com/pkg/errors"
)

func main() {
	var (
		flHostname = flag.String("hostname", "dababe.launcher.kolide.com:443", "")
	)
	flag.Parse()

	logger := log.NewLogfmtLogger(os.Stderr)
	logger = level.Info(logger)

	var (
		enrollSecret = env.String("KOLIDE_LAUNCHER_ENROLL_SECRET", "invalid_on_purpose")

		serverURL       = env.String("KOLIDE_LAUNCHER_HOSTNAME", *flHostname)
		insecureTLS     = env.Bool("KOLIDE_LAUNCHER_INSECURE", false)
		insecureGRPC    = env.Bool("KOLIDE_LAUNCHER_INSECURE_GRPC", false)
		loggingInterval = env.Duration("KOLIDE_LAUNCHER_LOGGING_INTERVAL", 60*time.Second)

		// TODO(future pr): these values are unset
		// they'll have to be parsed from a string
		certPins [][]byte
		rootPool *x509.CertPool
	)
	conn, err := service.DialGRPC(
		serverURL,
		insecureTLS,
		insecureGRPC,
		certPins,
		rootPool,
		logger,
	)
	if err != nil {
		logger.Log("err", err, "failed to connect to grpc host")
		os.Exit(1)
	}
	remote := service.New(conn, logger)

	extOpts := grpcext.ExtensionOpts{
		EnrollSecret:    enrollSecret,
		Logger:          logger,
		LoggingInterval: loggingInterval,
	}

	dbPath := "kolide_launcher_flare.db"
	defer os.Remove(dbPath)
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		logger.Log("err", errors.Wrap(err, "open local store"))
		os.Exit(1)
	}
	defer db.Close()

	ext, err := grpcext.NewExtension(remote, db, extOpts)
	if err != nil {
		logger.Log("err", errors.Wrap(err, "starting grpc extension"))
		os.Exit(1)
	}

	ext.SetQuerier(&queryier{})
	ctx := context.Background()
	_, _, err = ext.Enroll(ctx)
	if err != nil {
		logger.Log("err", errors.Wrap(err, "enrolling host"))
	}

	logger.Log("msg", "connecting to notary")
	req, err := http.NewRequest("GET", "https://notary.kolide.co/_notary_server/health", nil)
	if err != nil {
		logger.Log("err", err)
		os.Exit(1)
	}

	cctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := http.DefaultClient.Do(req.WithContext(cctx))
	if err != nil {
		logger.Log("err", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	logger.Log("msg", "connected to notary", "code", resp.Status)

}

type queryier struct {
}

func (q *queryier) Query(query string) ([]map[string]string, error) {
	return []map[string]string{
		map[string]string{},
	}, nil
}
