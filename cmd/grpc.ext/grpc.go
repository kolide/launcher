package main

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log/level"
	kolidelog "github.com/kolide/launcher/log"
	grpcext "github.com/kolide/launcher/osquery"
	"github.com/kolide/launcher/service"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/config"
	"github.com/kolide/osquery-go/plugin/distributed"
	osquery_logger "github.com/kolide/osquery-go/plugin/logger"
	"github.com/pkg/errors"
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

	logger := kolidelog.NewLogger(os.Stderr)
	if *flVerbose {
		logger.AllowDebug()
	}

	client, err := osquery.NewClient(*flSocketPath, timeout)
	if err != nil {
		logger.Fatal("err", err, "creating osquery extension client")
	}

	var conn *grpc.ClientConn
	var enrollSecret string
	remote := service.New(conn, level.Debug(logger))

	extOpts := grpcext.ExtensionOpts{
		EnrollSecret:    enrollSecret,
		Logger:          logger,
		LoggingInterval: 30 * time.Second,
	}

	var rootDirectory string
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
