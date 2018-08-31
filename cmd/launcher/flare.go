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
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/service"
	"github.com/pkg/errors"
)

func runFlare(args []string) error {
	flagset := flag.NewFlagSet("launcher flare", flag.ExitOnError)
	var (
		flHostname = flag.String("hostname", "dababe.launcher.kolide.com:443", "")

		// not documented via flags on purpose
		enrollSecret = env.String("KOLIDE_LAUNCHER_ENROLL_SECRET", "flare_ping")
		serverURL    = env.String("KOLIDE_LAUNCHER_HOSTNAME", *flHostname)
		insecureTLS  = env.Bool("KOLIDE_LAUNCHER_INSECURE", false)
		insecureGRPC = env.Bool("KOLIDE_LAUNCHER_INSECURE_GRPC", false)

		certPins [][]byte
		rootPool *x509.CertPool
	)
	flagset.Usage = commandUsage(flagset, "launcher flare")
	if err := flagset.Parse(args); err != nil {
		return err
	}

	id := ulid.New()
	b := new(bytes.Buffer)
	reportName := fmt.Sprintf("kolide_launcher_flare_report_%s", id)
	tarOut, err := os.Create(fmt.Sprintf("%s.tar.gz", reportName))
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
	output(b, stdout, string(jsonVersion))

	logger := log.NewLogfmtLogger(b)
	err = reportGRPCNetwork(
		logger,
		serverURL,
		insecureTLS,
		insecureGRPC,
		enrollSecret,
		certPins,
		rootPool,
	)
	output(b, stdout, "GRPC Connection ...%v\n", err == nil)
	if err != nil {
		output(b, fileOnly, "reportGRPCNetwork error: %s", err)
	}

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

// uses grpc to test connectivity. Does not depend on the osquery runtime for this test.
func reportGRPCNetwork(
	logger log.Logger,
	serverURL string,
	insecureTLS bool,
	insecureGRPC bool,
	enrollSecret string,
	certPins [][]byte,
	rootPool *x509.CertPool,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := service.DialGRPC(
		serverURL,
		insecureTLS,
		insecureGRPC,
		certPins,
		rootPool,
		logger,
	)

	if err != nil {
		return errors.Wrap(err, "establishing grpc connection to server")
	}
	remote := service.New(conn, logger)

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
