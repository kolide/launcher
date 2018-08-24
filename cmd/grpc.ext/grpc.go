package main

import (
	"context"
	"crypto/x509"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/config"
	"github.com/kolide/osquery-go/plugin/distributed"
	osquery_logger "github.com/kolide/osquery-go/plugin/logger"
	"github.com/pkg/errors"

	"github.com/kolide/launcher/pkg/log"
	grpcext "github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/service"
)

func main() {
	var (
		flSocketPath = flag.String("socket", "", "")
		flTimeout    = flag.Int("timeout", 0, "")
		flVerbose    = flag.Bool("verbose", false, "")
		_            = flag.Int("interval", 0, "")
	)
	flag.Parse()

	timeout := time.Duration(*flTimeout) * time.Second

	// allow for osqueryd to create the socket path
	time.Sleep(2 * time.Second)

	logdestination := os.Stderr
	if runtime.GOOS == "windows" {
		logdir := `C:\Windows\Temp\kolide`
		logfile := filepath.Join(logdir, "grpc.log")
		os.MkdirAll(logdir, os.ModeDir)
		f, err := os.Create(logfile)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer f.Close()
		logdestination = f
	}
	logger := log.NewLogger(logdestination)
	if *flVerbose {
		logger.AllowDebug()
	}

	client, err := osquery.NewClient(*flSocketPath, timeout)
	if err != nil {
		logger.Fatal("err", err, "creating osquery extension client")
	}

	var (
		enrollSecret  = env.String("KOLIDE_LAUNCHER_ENROLL_SECRET", "")
		rootDirectory = env.String("KOLIDE_LAUNCHER_ROOT_DIRECTORY", "")

		serverURL       = env.String("KOLIDE_LAUNCHER_HOSTNAME", "")
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
		logger.Fatal("err", err, "failed to connect to grpc host")
	}
	remote := service.New(conn, level.Debug(logger))

	extOpts := grpcext.ExtensionOpts{
		EnrollSecret:    enrollSecret,
		Logger:          logger,
		LoggingInterval: loggingInterval,
	}

	db, err := bolt.Open(filepath.Join(rootDirectory, "launcher.db"), 0600, nil)
	if err != nil {
		logger.Fatal("err", errors.Wrap(err, "open local store"))
	}
	defer db.Close()

	ext, err := grpcext.NewExtension(remote, db, extOpts)
	if err != nil {
		logger.Fatal("err", errors.Wrap(err, "starting grpc extension"))
	}

	// create an extension server
	server, err := osquery.NewExtensionManagerServer(
		"com.kolide.grpc_extension",
		*flSocketPath,
		osquery.ServerTimeout(timeout),
	)
	if err != nil {
		logger.Fatal("err", err, "msg", "creating osquery extension server")
	}

	configPlugin := config.NewPlugin("kolide_grpc", ext.GenerateConfigs)
	distributedPlugin := distributed.NewPlugin("kolide_grpc", ext.GetQueries, ext.WriteResults)
	loggerPlugin := osquery_logger.NewPlugin("kolide_grpc", ext.LogString)

	server.RegisterPlugin(configPlugin, distributedPlugin, loggerPlugin)

	ext.SetQuerier(&queryier{client: client})
	ctx := context.Background()
	_, invalid, err := ext.Enroll(ctx)
	if err != nil {
		logger.Fatal("err", errors.Wrap(err, "enrolling host"))
	}
	if invalid {
		logger.Fatal(errors.Wrap(err, "invalid enroll secret"))
	}
	ext.Start()
	defer ext.Shutdown()

	ext.Start()
	defer ext.Shutdown()

	if err := server.Run(); err != nil {
		logger.Fatal("err", err)
	}
}

type queryier struct {
	client *osquery.ExtensionManagerClient
}

func (q *queryier) Query(query string) ([]map[string]string, error) {
	resp, err := q.client.Query(query)
	if err != nil {
		return nil, errors.Wrap(err, "could not query the extension manager client")
	}
	if resp.Status.Code != int32(0) {
		return nil, errors.New(resp.Status.Message)
	}
	return resp.Response, nil
}
