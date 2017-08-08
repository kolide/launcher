package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"pault.ag/go/debian/deb"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	xar "github.com/groob/goxar"
	"github.com/kolide/launcher/tools/packaging"
	"github.com/pkg/errors"
)

type mirror struct {
	path           string
	logger         log.Logger
	platform       string
	osqueryVersion string
	updateChannel  string
}

// downloads the stable version of osquery for a given platform
// valid values are darwin,linux,windows
func (m *mirror) downloadOsqueryPackage() error {
	destination := filepath.Join(m.path, m.platform)
	if err := os.MkdirAll(destination, packaging.DirMode); err != nil {
		return errors.Wrapf(err, "create %s folder for osquery downloads", destination)
	}
	var url string
	switch m.platform {
	case "darwin":
		url = "https://osquery-packages.s3.amazonaws.com/darwin/osquery.pkg"
		destination = filepath.Join(destination, "osquery.pkg")
	case "linux":
		// using rpm because it's a simple tar archive.
		url = "https://osquery-packages.s3.amazonaws.com/xenial/osquery.deb"
		destination = filepath.Join(destination, "osquery.deb")
	case "windows":
		// TODO
		// https://github.com/kolide/launcher/issues/71
		// The windows URL is https://chocolatey.org/api/v2/package/osquery/<version>,
		// and https://osquery-packages.s3.amazonaws.com/windows/osquery.nupkg
	default:
		return fmt.Errorf("unknown platform for download url %s", m.platform)
	}

	level.Debug(m.logger).Log(
		"msg", "downloading osquery package",
		"platform", m.platform,
		"download_url", url,
		"package_path", destination,
	)

	resp, err := http.Get(url)
	if err != nil {
		return errors.Wrapf(err, "downloading osquery package for %s from url %s", m.platform, url)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected http status %s when downloading osquery package %s", resp.Status, url)
	}
	defer resp.Body.Close()

	out, err := os.OpenFile(destination, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return errors.Wrapf(err, "open or create file %s", destination)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return errors.Wrap(err, "save downloaded osquery package to file")
	}

	level.Info(m.logger).Log(
		"msg", "downloaded new osquery package",
		"package_path", destination,
	)

	return nil
}

func (m *mirror) extractLinux() error {
	savePath := filepath.Join(m.path, m.platform, "bin", "osqueryd")
	if err := os.MkdirAll(filepath.Dir(savePath), packaging.DirMode); err != nil {
		return err
	}
	out, err := os.OpenFile(savePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	pkgPath := filepath.Join(m.path, m.platform, "osquery.deb")
	debFile, closer, err := deb.LoadFile(pkgPath)
	if err != nil {
		return err
	}
	defer closer()

	tr := debFile.Data

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Name == "./usr/bin/osqueryd" {
			if _, err := io.CopyN(out, tr, header.Size); err != nil {
				return err
			}
			break
		}
	}

	return nil

}

func (m *mirror) extract() error {
	switch m.platform {
	case "darwin":
		return m.extractDarwin()
	case "linux":
		return m.extractLinux()
	default:
		return fmt.Errorf("unsupported platform %s", m.platform)
	}
}

func (m *mirror) extractDarwin() error {
	pkgPath := filepath.Join(m.path, m.platform, "osquery.pkg")
	xr, err := xar.OpenReader(pkgPath)
	if err != nil {
		return errors.Wrapf(err, "opening xar archive %s", pkgPath)
	}
	var payload *xar.File
	for _, file := range xr.File {
		if file.Name == "Payload" {
			payload = file
			break
		}
	}
	if payload == nil {
		return fmt.Errorf("pkg archive %s missing Payload file", pkgPath)
	}
	level.Debug(m.logger).Log(
		"msg", "reading Payload file in osquery pkg",
		"package_path", pkgPath,
		"mime_type", payload.EncodingMimetype,
	)

	file, err := payload.OpenRaw()
	if err != nil {
		return errors.Wrap(err, "opening Payload File from mac pkg")
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return errors.Wrap(err, "Payload file not a gzip archive")
	}

	if err := m.readCpioFile(gzr, "./usr/local/bin/osqueryd"); err != nil {
		return err
	}

	return nil
}

// executes the cpio command to extract a file from the stream.
// using the exec package, because none of the Go libraries support
// the odc(070707) header yet.
func (m *mirror) readCpioFile(input io.Reader, filename string) error {
	tempdir, err := ioutil.TempDir("", "")
	if err != nil {
		return errors.Wrap(err, "create TempDir for cpio")
	}
	defer os.RemoveAll(tempdir)
	cmd := exec.Command("cpio", "-ivdm", filename)
	cmd.Dir = tempdir
	cmd.Stdin = input
	level.Debug(m.logger).Log(
		"msg", "executing cpio command",
		"cmd", strings.Join(cmd.Args, " "),
	)
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "run command %v", cmd.Args)
	}
	tmpbin := filepath.Join(tempdir, filepath.Clean(filename))
	newbin := filepath.Join(m.path, m.platform, "bin", filepath.Base(filename))
	if err := os.MkdirAll(filepath.Dir(newbin), packaging.DirMode); err != nil {
		return errors.Wrapf(err, "create directory %s", filepath.Dir(newbin))
	}
	level.Debug(m.logger).Log(
		"msg", "copying extracted file",
		"src", tmpbin,
		"dst", newbin,
	)
	return os.Rename(tmpbin, newbin)
}

// runs osqueryd --version to find out the current osquery version
// requires that the binary for the current platform.
func (m *mirror) determineOsqueryVersion() error {
	platform := runtime.GOOS
	binPath := filepath.Join(m.path, platform, "bin", "osqueryd")
	cmd := exec.Command(binPath, "--version")
	level.Debug(m.logger).Log(
		"msg", "executing osqueryd command",
		"cmd", strings.Join(cmd.Args, " "),
	)
	out, err := cmd.Output()
	if err != nil {
		return errors.Wrap(err, "determine osqueryd version")
	}
	versionSlice := bytes.Split(out, []byte(" "))
	version := string(bytes.TrimSpace(versionSlice[len(versionSlice)-1]))
	level.Debug(m.logger).Log(
		"msg", "found osquery version",
		"full_string", string(out),
		"parsed_version", version,
	)
	m.osqueryVersion = version
	return nil
}

// name of GCS bucket where tarballs are saved to
const mirrorBucketname = "binaries-for-launcher"

// wraps a single file in a tarball
func (m *mirror) createTarball(source string) error {
	saveDir := filepath.Join(m.path, mirrorBucketname, "kolide", m.platform)
	if err := os.MkdirAll(saveDir, packaging.DirMode); err != nil {
		return errors.Wrapf(err, "create tarball directory %s", saveDir)
	}
	if m.osqueryVersion == "" {
		if err := m.determineOsqueryVersion(); err != nil {
			return err
		}
	}
	filename := fmt.Sprintf("%s-%s.tar.gz", filepath.Base(source), m.osqueryVersion)
	savePath := filepath.Join(saveDir, filename)
	level.Debug(m.logger).Log(
		"msg", "creating tarball",
		"output", savePath,
	)

	f, err := os.Create(savePath)
	if err != nil {
		return errors.Wrapf(err, "create filepath %s", savePath)
	}
	defer f.Close()

	gzw := gzip.NewWriter(f)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	level.Debug(m.logger).Log(
		"msg", "adding file to tarball",
		"output", savePath,
		"source", source,
	)

	file, err := os.Open(source)
	if err != nil {
		return errors.Wrapf(err, "open file %s", source)
	}

	info, err := file.Stat()
	if err != nil {
		return errors.Wrapf(err, "could not stat", source)
	}

	hdr, err := tar.FileInfoHeader(info, "osqueryd")
	if err != nil {
		return errors.Wrap(err, "create tar header")
	}

	if err := tw.WriteHeader(hdr); err != nil {
		return errors.Wrap(err, "write osqueryd tar header")
	}

	if _, err := io.Copy(tw, file); err != nil {
		return errors.Wrap(err, "copy file to tar writer")
	}

	level.Debug(m.logger).Log(
		"msg", "saved tarball",
		"output", savePath,
	)

	return nil
}

func (m *mirror) createTaggedTarball(source string) error {
	saveDir := filepath.Join(m.path, mirrorBucketname, "kolide", m.platform)
	filename := fmt.Sprintf("%s-%s.tar.gz", filepath.Base(source), m.osqueryVersion)
	versionTarball := filepath.Join(saveDir, filename)
	taggedFilename := fmt.Sprintf("%s-%s.tar.gz", filepath.Base(source), m.updateChannel)
	taggedTarballPath := filepath.Join(saveDir, taggedFilename)
	level.Debug(m.logger).Log(
		"msg", "create tagged tarball",
		"output", taggedTarballPath,
	)

	if err := packaging.CopyFile(versionTarball, taggedTarballPath); err != nil {
		return errors.Wrapf(err, "copy file %s to %s", versionTarball, taggedTarballPath)
	}

	return nil
}

func runMirror(args []string) error {
	flagset := flag.NewFlagSet("mirror", flag.ExitOnError)
	var (
		flDebug = flagset.Bool(
			"debug",
			false,
			"enable debug logging",
		)
		flDownload = flagset.Bool(
			"download",
			true,
			"download a fresh copy of osquery from s3",
		)
		flExtract = flagset.Bool(
			"extract",
			true,
			"extract binary from downloaded archive",
		)
		flTar = flagset.Bool(
			"tar",
			true,
			"create osqueryd.tar.gz archive from binary",
		)
		flPlatform = flagset.String(
			"platform",
			"darwin",
			"platform to mirror packages for. Valid values: darwin,linux,windows.",
		)
		flUpdateChannel = flagset.String(
			"update_channel",
			"stable",
			"create a tarball for a specific autoupdate channel. Valid values: stable,beta,nightly",
		)
	)
	flagset.Usage = usageFor(flagset, "package-builder mirror [flags]")
	if err := flagset.Parse(args); err != nil {
		return err
	}

	logger := log.NewJSONLogger(os.Stderr)
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.Caller(5))

	if *flDebug {
		logger = level.NewFilter(logger, level.AllowDebug())
	} else {
		logger = level.NewFilter(logger, level.AllowInfo())
	}

	m := &mirror{
		path:          "/tmp/osquery_mirror",
		logger:        logger,
		platform:      *flPlatform,
		updateChannel: *flUpdateChannel,
	}

	if *flDownload {
		if err := m.downloadOsqueryPackage(); err != nil {
			return err
		}
	}

	if *flExtract && *flPlatform == "darwin" {
		if err := m.extract(); err != nil {
			return err
		}
	}
	if *flExtract && *flPlatform == "linux" {
		// TODO move to an extract helper with a platform switch statement
		if err := m.extractLinux(); err != nil {
			return err
		}
	}

	if err := m.determineOsqueryVersion(); err != nil {
		return err
	}

	if *flTar {
		source := filepath.Join(m.path, m.platform, "bin", "osqueryd")
		if err := m.createTarball(source); err != nil {
			return err
		}
		if m.updateChannel != "" {
			if err := m.createTaggedTarball(source); err != nil {
				return err
			}
		}
	}

	return nil
}
