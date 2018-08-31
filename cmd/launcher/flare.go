package main

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/kit/version"
)

func runFlare(args []string) error {
	flagset := flag.NewFlagSet("launcher flare", flag.ExitOnError)
	var ()
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
