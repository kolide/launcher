package main

import (
	"context"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/version"
	grpcext "github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/service"
	osquery "github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/config"
	"github.com/osquery/osquery-go/plugin/distributed"
	osquery_logger "github.com/osquery/osquery-go/plugin/logger"
	"go.etcd.io/bbolt"
)

func main() {
	var (
		flSocketPath = flag.String("socket", "", "")
		flTimeout    = flag.Int("timeout", 2, "")
		flVerbose    = flag.Bool("verbose", false, "")
		flVersion    = flag.Bool("version", false, "Print Launcher version and exit")

		_ = flag.Int("interval", 0, "")
	)
	flag.Parse()

	if *flVersion {
		version.PrintFull()
		os.Exit(0)
	}

	timeout := time.Duration(*flTimeout) * time.Second

	// allow for osqueryd to create the socket path
	time.Sleep(2 * time.Second)

	logger := logutil.NewServerLogger(*flVerbose)

	client, err := osquery.NewClient(*flSocketPath, timeout)
	if err != nil {
		logutil.Fatal(logger, "err", err, "creating osquery extension client", "stack", fmt.Sprintf("%+v", err))
	}

	var (
		enrollSecret  = env.String("KOLIDE_LAUNCHER_ENROLL_SECRET", "")
		rootDirectory = env.String("KOLIDE_LAUNCHER_ROOT_DIRECTORY", "")

		serverURL         = env.String("KOLIDE_LAUNCHER_HOSTNAME", "")
		insecureTLS       = env.Bool("KOLIDE_LAUNCHER_INSECURE", false)
		insecureTransport = env.Bool("KOLIDE_LAUNCHER_INSECURE_TRANSPORT", false)
		loggingInterval   = env.Duration("KOLIDE_LAUNCHER_LOGGING_INTERVAL", 60*time.Second)

		// TODO(future pr): these values are unset
		// they'll have to be parsed from a string
		certPins [][]byte
		rootPool *x509.CertPool
	)
	conn, err := service.DialGRPC(
		serverURL,
		insecureTLS,
		insecureTransport,
		certPins,
		rootPool,
		logger,
	)
	if err != nil {
		logutil.Fatal(logger, "err", err, "failed to connect to grpc host", "stack", fmt.Sprintf("%+v", err))
	}
	remote := service.NewGRPCClient(conn, level.Debug(logger))

	extOpts := grpcext.ExtensionOpts{
		EnrollSecret:    enrollSecret,
		Logger:          logger,
		LoggingInterval: loggingInterval,
	}

	db, err := bbolt.Open(filepath.Join(rootDirectory, "launcher.db"), 0600, nil)
	if err != nil {
		logutil.Fatal(logger, "err", fmt.Errorf("open local store: %w", err), "stack", fmt.Sprintf("%+v", err))
	}
	defer db.Close()

	ext, err := grpcext.NewExtension(remote, db, extOpts)
	if err != nil {
		logutil.Fatal(logger, "err", fmt.Errorf("starting grpc extension: %w", err), "stack", fmt.Sprintf("%+v", err))
	}

	// create an extension server
	server, err := osquery.NewExtensionManagerServer(
		"com.kolide.grpc_extension",
		*flSocketPath,
		osquery.ServerTimeout(timeout),
	)
	if err != nil {
		logutil.Fatal(logger, "err", err, "msg", "creating osquery extension server", "stack", fmt.Sprintf("%+v", err))
	}

	configPlugin := config.NewPlugin("kolide_grpc", ext.GenerateConfigs)
	distributedPlugin := distributed.NewPlugin("kolide_grpc", ext.GetQueries, ext.WriteResults)
	loggerPlugin := osquery_logger.NewPlugin("kolide_grpc", ext.LogString)

	server.RegisterPlugin(configPlugin, distributedPlugin, loggerPlugin)

	ext.SetQuerier(&queryier{client: client})
	ctx := context.Background()
	_, invalid, err := ext.Enroll(ctx)
	if err != nil {
		logutil.Fatal(logger, "err", fmt.Errorf("enrolling host: %w", err), "stack", fmt.Sprintf("%+v", err))
	}
	if invalid {
		logutil.Fatal(logger, fmt.Errorf("invalid enroll secret: %w", err), "stack", fmt.Sprintf("%+v", err))
	}
	ext.Start()
	defer ext.Shutdown()

	ext.Start()
	defer ext.Shutdown()

	if err := server.Run(); err != nil {
		logutil.Fatal(logger, "err", err, "stack", fmt.Sprintf("%+v", err))
	}
}

type queryier struct {
	client *osquery.ExtensionManagerClient
}

func (q *queryier) Query(query string) ([]map[string]string, error) {
	resp, err := q.client.Query(query)
	if err != nil {
		return nil, fmt.Errorf("could not query the extension manager client: %w", err)
	}
	if resp.Status.Code != int32(0) {
		return nil, errors.New(resp.Status.Message)
	}
	return resp.Response, nil
}
