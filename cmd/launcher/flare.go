package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/agent/flags"
	"github.com/kolide/launcher/pkg/agent/knapsack"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/osquery/runtime"
	"github.com/kolide/launcher/pkg/service"
	osquerygo "github.com/osquery/osquery-go"
)

func runFlare(args []string) error {
	// Flare assumes a launcher installation (at least partially) exists
	// Overriding some of the default values allows options to be parsed making this assumption
	defaultKolideHosted = true
	defaultAutoupdate = true
	setDefaultPaths()

	opts, err := parseOptions("flare", args)
	if err != nil {
		return err
	}

	var (
		flHostname = flag.String("hostname", "dababe.launcher.kolide.com:443", "")

		// not documented via flags on purpose
		enrollSecret      = env.String("KOLIDE_LAUNCHER_ENROLL_SECRET", "flare_ping")
		serverURL         = env.String("KOLIDE_LAUNCHER_HOSTNAME", *flHostname)
		insecureTLS       = env.Bool("KOLIDE_LAUNCHER_INSECURE", false)
		insecureTransport = env.Bool("KOLIDE_LAUNCHER_INSECURE_TRANSPORT", false)
		flareSocketPath   = env.String("FLARE_SOCKET_PATH", agent.TempPath("flare.sock"))
		tarDirPath        = env.String("KOLIDE_LAUNCHER_FLARE_TAR_DIR_PATH", "")

		certPins [][]byte
		rootPool *x509.CertPool
	)

	id := ulid.New()
	b := new(bytes.Buffer)
	reportName := fmt.Sprintf("kolide_launcher_flare_report_%s", id)
	reportPath := fmt.Sprintf("%s.tar.gz", filepath.Join(tarDirPath, reportName))
	output(b, stdout, fmt.Sprintf("Generating flare report file: %s\n", reportPath))

	tarOut, err := os.Create(reportPath)
	if err != nil {
		fatal(b, err)
	}
	defer func() {
		if err := tarOut.Close(); err != nil {
			fatal(b, err)
		}
	}()

	tw := tar.NewWriter(tarOut)

	// create directory at root of tar file
	baseDir := filepath.ToSlash(reportName)
	hdr := &tar.Header{
		Name:     baseDir + "/",
		Mode:     0755,
		ModTime:  time.Now().UTC(),
		Typeflag: tar.TypeDir,
	}

	if err := tw.WriteHeader(hdr); err != nil {
		fatal(b, err)
	}

	defer func() {
		hdr := &tar.Header{
			Name: filepath.Join(baseDir, fmt.Sprintf("%s.log", id)),
			Mode: int64(os.ModePerm),
			Size: int64(b.Len()),
		}

		if err := tw.WriteHeader(hdr); err != nil {
			fatal(b, err)
		}

		if _, err := tw.Write(b.Bytes()); err != nil {
			fatal(b, err)
		}

		if err := tw.Close(); err != nil {
			fatal(b, err)
		}
	}()

	output(b, stdout, "Starting Launcher Diagnostics\n")
	output(b, stdout, "ID: %s\n", id)
	user, err := user.Current()
	if err != nil {
		fatal(b, err)
	}
	output(b, stdout, "CurrentUser: %s uid: %s\n", user.Username, user.Uid)
	v := version.Version()
	jsonVersion, err := json.Marshal(&v)
	if err != nil {
		fatal(b, err)
	}
	output(b, stdout, "%v\n", string(jsonVersion))

	logger := log.NewLogfmtLogger(b)
	fcOpts := []flags.Option{flags.WithCmdLineOpts(opts)}
	flagController := flags.NewFlagController(logger, nil, fcOpts...)
	k := knapsack.New(nil, flagController, nil)

	output(b, stdout, "\nStarting Launcher Doctor\n")
	// Run doctor but disable color output since this is being directed to a file
	os.Setenv("NO_COLOR", "1")
	buildAndRunCheckups(logger, k, opts, b)
	output(b, stdout, "\nEnd of Launcher Doctor\n")

	err = reportGRPCNetwork(
		logger,
		serverURL,
		insecureTLS,
		insecureTransport,
		enrollSecret,
		certPins,
		rootPool,
	)
	output(b, stdout, "GRPC Connection ...%v\n", err == nil)
	if err != nil {
		output(b, fileOnly, "reportGRPCNetwork error: %s", err)
	}

	err = reportOsqueryProcessInfo(logger, flareSocketPath, b)
	if err != nil {
		output(b, fileOnly, "reportOsqueryProcessInfo error: %s", err)
	}
	output(b, stdout, "Osqueryi ProcessInfo ...%v\n", err == nil)

	err = reportNotaryPing(logger)
	if err != nil {
		output(b, fileOnly, "reportNotaryPing error: %s", err)
	}
	output(b, stdout, "Osqueryi Ping Notary ...%v\n", err == nil)

	return nil
}

type outputDestination int

const (
	fileOnly outputDestination = iota
	stdout
)

func fatal(w io.Writer, err error) {
	output(w, stdout, "error: %s\n", err)
	os.Exit(1)
}

func output(w io.Writer, printTo outputDestination, f string, a ...interface{}) error {
	if printTo == stdout {
		fmt.Printf(f, a...)
	}

	_, err := fmt.Fprintf(w, f, a...)
	return err
}

// starts an osqueryd runtime, and then connects an osquery client and runs queries to check and log process info.
func reportOsqueryProcessInfo(
	logger log.Logger,
	socketPath string,
	output io.Writer,
) error {
	logger.Log(
		"flare", "reportOsqueryProcessInfo",
		"msg", "creating osquery runner",
		"socketPath", socketPath,
	)
	// create the osquery runtime socket directory
	if _, err := os.Stat(filepath.Dir(socketPath)); os.IsNotExist(err) {
		if err := os.Mkdir(filepath.Dir(socketPath), fsutil.DirMode); err != nil {
			return fmt.Errorf("creating socket path base directory: %w", err)
		}
	}

	opts := []runtime.OsqueryInstanceOption{
		runtime.WithExtensionSocketPath(socketPath),
	}

	defaultBinaryPath := "/usr/local/kolide/bin/osqueryd"
	if _, err := os.Stat(defaultBinaryPath); err == nil {
		// try to use the default binary location. Can improve on this in the future by checking launchd/systemd
		// for the value in the package.
		// if dfault path not found, will default to PATH.
		opts = append(opts, runtime.WithOsquerydBinary(defaultBinaryPath))
	}

	// start a osquery runtime
	runner, err := runtime.LaunchInstance(opts...)
	if err != nil {
		return fmt.Errorf("creating osquery instance for process info query: %w", err)
	}
	defer func() {
		if err := runner.Shutdown(); err != nil {
			logger.Log(
				"msg", "shutting down runner from reportOsqueryProcessInfo",
				"err", err,
			)
		}
	}()

	logger.Log(
		"flare", "reportOsqueryProcessInfo",
		"msg", "creating osquery-go client",
		"socketPath", socketPath,
	)

	// start a client and query it
	client, err := osquerygo.NewClient(socketPath, 5*time.Second)
	if err != nil {
		return fmt.Errorf("creating osquerygo client with socket path %s: %w", socketPath, err)
	}
	defer client.Close()

	logger.Log(
		"flare", "reportOsqueryProcessInfo",
		"msg", "running query with osquery-go",
	)

	const query = `select * from processes where name like '%osqueryd%' OR name like '%launcher%';`
	resp, err := client.Query(query)
	if err != nil {
		return fmt.Errorf("running osquery query for process info: %w", err)
	}

	logger.Log(
		"flare", "reportOsqueryProcessInfo",
		"status_code", resp.Status.Code,
	)

	if resp.Status.Code != int32(0) {
		return fmt.Errorf("Error running query: %s", resp.Status.Message)
	}

	results := struct {
		Results map[string]interface{} `json:"osquery_results"`
	}{
		Results: map[string]interface{}{},
	}
	results.Results["process_info"] = resp.Response

	enc := json.NewEncoder(output)
	enc.SetIndent("", "  ")
	if err := enc.Encode(results); err != nil {
		return fmt.Errorf("encoding JSON query results: %w", err)
	}

	return nil
}

// uses grpc to test connectivity. Does not depend on the osquery runtime for this test.
func reportGRPCNetwork(
	logger log.Logger,
	serverURL string,
	insecureTLS bool,
	insecureTransport bool,
	enrollSecret string,
	certPins [][]byte,
	rootPool *x509.CertPool,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := service.DialGRPC(
		serverURL,
		insecureTLS,
		insecureTransport,
		certPins,
		rootPool,
		logger,
	)

	if err != nil {
		return fmt.Errorf("establishing grpc connection to server: %w", err)
	}
	remote := service.NewGRPCClient(conn, logger)

	logger.Log(
		"flare", "reportGRPCNetwork",
		"msg", "attempting RequestConfig with invalid nodeKey",
		"server_url", serverURL,
	)

	config, invalid, err := remote.RequestConfig(ctx, "flare_ping")
	logger.Log(
		"flare", "reportGRPCNetwork",
		"msg", "done with RequestConfig",
		"server_url", serverURL,
		"err", err,
		"invalid", invalid,
		"config", config,
	)

	nodeKey, invalid, err := remote.RequestEnrollment(
		ctx, enrollSecret, "flare_host", service.EnrollmentDetails{Hostname: "flare_host"},
	)
	logger.Log(
		"flare", "reportGRPCNetwork",
		"msg", "done with RequestEnrollment",
		"server_url", serverURL,
		"invalid", invalid,
		"err", err,
		"nodeKey", nodeKey,
	)

	return nil
}

func reportNotaryPing(
	logger log.Logger,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	notaryURL, _ := url.Parse(autoupdate.DefaultNotary)
	notaryURL.Path = "/_notary_server/health"
	req, err := http.NewRequest(http.MethodGet, notaryURL.String(), nil)
	if err != nil {
		return fmt.Errorf("create http request to %s: %w", notaryURL, err)
	}
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	keyvals := []interface{}{
		"flare", "reportNotaryPing",
		"msg", "ping notary server",
		"server_url", notaryURL,
	}
	if err != nil {
		keyvals = append(keyvals, "err", err)
	} else {
		keyvals = append(keyvals, "response_code", resp.StatusCode)
	}
	defer resp.Body.Close()
	logger.Log(keyvals...)
	return nil
}
