package mirror

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	xar "github.com/groob/goxar"
	kv "github.com/kolide/kit/version"
	"github.com/kolide/launcher/tools/packaging"

	"github.com/pkg/errors"
	"pault.ag/go/debian/deb"
)

// ErrNotSupported thrown for unsupported platforms such as windows
var ErrNotSupported = errors.New("platform not supported")

// name of GCS bucket where tarballs are saved to
const (
	mirrorBucketname = "binaries-for-launcher"
	stagingPath      = "/tmp/osquery_mirror"

	osqueryLinuxSourceURL  = "https://osquery-packages.s3.amazonaws.com/xenial/osquery.deb"
	osqueryDarwinSourceURL = "https://osquery-packages.s3.amazonaws.com/darwin/osquery.pkg"

	propPlatform                   = "platform"
	propChannel                    = "channel"
	propOsqueryVersion             = "version"
	propOsqueryVersionTarballPath  = "osquery-version-tarball-path"
	propOsqueryTaggedTarballPath   = "osquery-tagged-tarball-path"
	propLauncherVersionTarballPath = "launcher-version-tarball-path"
	propLauncherTaggedTarballPath  = "launcher-tagged-tarball-path"

	PlatformLinux   = "linux"
	PlatformDarwin  = "darwin"
	PlatformWindows = "windows"

	ChannelStable  = "stable"
	ChannelNightly = "nightly"
	ChannelBeta    = "beta"

	orderDownload int = iota
	orderExtract
	orderOsqueryVersion
	orderOsqueryTarball
	orderOsqueryTaggedTarball
	orderOsqueryMirrorUpload
	orderOsqueryNotaryPublish
	orderLauncherTarball
	orderLauncherUpload
	orderLauncherPublish
)

// Flags determine which mirror operations will be executed
type Flags struct {
	// Download osquery from osquery project mirror.
	Download *bool
	// Extract osquery binary from package
	Extract *bool
	// OsqueryTarball create versioned and channel tarballs for osquery
	OsqueryTarball *bool
	// OsqeuryMirrorUpload upload osquery tarballs to mirror
	OsqueryMirrorUpload *bool
	// OsqueryNotaryPublish publishes mirrored binary information to Notary
	OsqueryNotaryPublish *bool
	// LauncherTarball created versioned and channel tarballs for Launcher
	LauncherTarball *bool
	// LauncherUpload upload launcher tarballs to mirror
	LauncherUpload *bool
	// LauncherPublish publish mirror updates to Notary
	LauncherPublish *bool
	// Platform is the target platform for the binaries we're working with
	Platform *string
	// Channel is the autoupdate channel, stable, beta etc.
	Channel *string
}

// Publish executes mirror actions defined by flags.
func Publish(logger log.Logger, flags Flags) error {
	props := properties{}
	props[propPlatform] = *flags.Platform
	props[propChannel] = *flags.Channel

	var root *node
	if *flags.LauncherTarball {
		add(
			&root,
			&node{
				order:  orderLauncherTarball,
				logger: logger,
				desc:   "create launcher tarball",
				accept: createLauncherTarballs,
			},
		)
	}
	if *flags.LauncherUpload {
		add(
			&root,
			&node{
				order:  orderLauncherUpload,
				pred:   intPtr(orderLauncherTarball),
				logger: logger,
				desc:   "upload launcher tarballs to mirror",
				accept: uploadLauncherTarballs,
			},
		)
	}
	if *flags.LauncherPublish {
		add(
			&root,
			&node{
				order:  orderLauncherPublish,
				pred:   intPtr(orderLauncherUpload),
				logger: logger,
				desc:   "publish launcher changes to notary",
				accept: publishLauncherToNotary,
			},
		)
	}
	if *flags.Download {
		add(
			&root,
			&node{
				order:  orderDownload,
				logger: logger,
				desc:   "downloading osquery",
				accept: downloadOsquery,
			},
		)
	}
	if *flags.Extract {
		add(
			&root,
			&node{
				order:  orderExtract,
				logger: logger,
				pred:   intPtr(orderDownload),
				desc:   "extract osquery",
				accept: extractOsquery,
			},
		)
	}
	if *flags.OsqueryTarball {
		add(
			&root,
			&node{
				order:  orderOsqueryVersion,
				logger: logger,
				pred:   intPtr(orderExtract),
				desc:   "get osquery version",
				accept: getOsqueryVersion,
			},
		)
		add(
			&root,
			&node{
				order:  orderOsqueryTarball,
				logger: logger,
				pred:   intPtr(orderOsqueryVersion),
				desc:   "generate osquery tarball",
				accept: createOsqueryTarballs,
			},
		)
		add(
			&root,
			&node{
				order:  orderOsqueryTaggedTarball,
				logger: logger,
				pred:   intPtr(orderOsqueryTarball),
				desc:   "generated osquery tagged tarball",
				accept: createOsqueryTaggedTarballs,
			},
		)
	}
	if *flags.OsqueryMirrorUpload {
		add(
			&root,
			&node{
				order:  orderOsqueryMirrorUpload,
				logger: logger,
				pred:   intPtr(orderOsqueryTaggedTarball),
				desc:   "upload osquery tarballs to mirror",
				accept: uploadOsqueryToMirror,
			},
		)
	}
	if *flags.OsqueryNotaryPublish {
		add(
			&root,
			&node{
				order:  orderOsqueryNotaryPublish,
				logger: logger,
				pred:   intPtr(orderOsqueryMirrorUpload),
				desc:   "publish new osquery binaries to notary",
				accept: publishOsqueryToNotary,
			},
		)
	}

	// unlikely but check just in case
	if err := hasCycle(root); err != nil {
		level.Error(logger).Log(
			"method", "Publish",
			"err", err,
		)
		return err
	}
	// execute each step in order
	if err := visit(root, &predTable{}, &props); err != nil {
		level.Error(logger).Log(
			"method", "Publish",
			"err", err,
		)
		return err
	}
	return nil
}

func intPtr(v int) *int { return &v }

// ToggleAllOperations sets all operations, uploads, publishing etc to true. Channel
// and Platform settings are retained.
func ToggleAllOperations(f Flags) Flags {
	b := true
	pTrue := &b

	return Flags{
		Platform:             f.Platform,
		Channel:              f.Channel,
		Extract:              pTrue,
		OsqueryTarball:       pTrue,
		OsqueryMirrorUpload:  pTrue,
		OsqueryNotaryPublish: pTrue,
		LauncherTarball:      pTrue,
		LauncherPublish:      pTrue,
		LauncherUpload:       pTrue,
		Download:             pTrue,
	}
}

// ToggleAllOsquery sets flags to perform all operations required to download and publish Osquery.
func ToggleAllOsquery(f Flags) Flags {
	b := true
	pTrue := &b
	return Flags{
		Platform:             f.Platform,
		Channel:              f.Channel,
		OsqueryTarball:       pTrue,
		OsqueryMirrorUpload:  pTrue,
		OsqueryNotaryPublish: pTrue,
		Extract:              pTrue,
		Download:             pTrue,
	}
}

// ToggleAllLauncher set flags to perform all operations required publish Launcher.
func ToggleAllLauncher(f Flags) Flags {
	b := true
	pTrue := &b
	return Flags{
		Platform:        f.Platform,
		Channel:         f.Channel,
		LauncherTarball: pTrue,
		LauncherPublish: pTrue,
		LauncherUpload:  pTrue,
	}
}

func getOsqueryVersion(n *node, props *properties) error {
	level.Debug(n.logger).Log(
		"msg", n.desc,
	)
	platform := runtime.GOOS
	binPath := filepath.Join(stagingPath, platform, "bin", "osqueryd")
	cmd := exec.Command(binPath, "--version")
	level.Debug(n.logger).Log(
		"msg", "executing osqueryd command",
		"cmd", strings.Join(cmd.Args, " "),
	)
	out, err := cmd.Output()
	if err != nil {
		return errors.Wrap(err, "determine osqueryd version")
	}
	versionSlice := bytes.Split(out, []byte(" "))
	version := string(bytes.TrimSpace(versionSlice[len(versionSlice)-1]))
	level.Debug(n.logger).Log(
		"msg", "found osquery version",
		"full_string", string(out),
		"parsed_version", version,
	)
	props.set(propOsqueryVersion, version)
	level.Info(n.logger).Log(
		"msg", n.desc,
		"version", version,
	)
	return nil
}

func downloadOsquery(n *node, props *properties) error {
	level.Debug(n.logger).Log(
		"msg", n.desc,
	)
	var platforms []string
	platform, err := props.getString(propPlatform)
	if err != nil {
		return err
	}
	platforms = append(platforms, platform)
	// we always need to download the osquery for the platform that we are on in order
	// to get osquery version
	if platform != runtime.GOOS {
		platforms = append(platforms, platform)
	}
	for _, currPlatform := range platforms {
		if err := downloadOsqueryByPlatform(currPlatform); err != nil {
			return err
		}
	}
	level.Info(n.logger).Log(
		"msg", fmt.Sprintf("completed %s", n.desc),
	)
	return nil
}

func extractOsquery(n *node, props *properties) error {
	level.Debug(n.logger).Log(
		"msg", n.desc,
	)
	var platforms []string
	platform, err := props.getString(propPlatform)
	if err != nil {
		return err
	}
	// We always need to get osquery for the platform we are on as well as the
	// target platform so we can run osquery to get its version.
	platforms = append(platforms, platform)
	if platform != runtime.GOOS {
		platforms = append(platforms, runtime.GOOS)
	}
	for _, currPlatform := range platforms {
		switch currPlatform {
		case PlatformDarwin:
			if err := extractDarwin(n.logger); err != nil {
				return err
			}
		case PlatformLinux:
			if err := extractLinux(n.logger); err != nil {
				return err
			}
		default:
			return ErrNotSupported
		}
	}
	level.Info(n.logger).Log(
		"msg", fmt.Sprintf("completed %s", n.desc),
	)
	return nil
}

func createOsqueryTarballs(n *node, props *properties) error {
	level.Debug(n.logger).Log(
		"msg", n.desc,
	)
	platform, err := props.getString(propPlatform)
	if err != nil {
		return err
	}
	version, err := props.getString(propOsqueryVersion)
	if err != nil {
		return err
	}
	saveDir := filepath.Join(stagingPath, mirrorBucketname, "kolide", "osqueryd", platform)
	if err := os.MkdirAll(saveDir, packaging.DirMode); err != nil {
		return errors.Wrapf(err, "create tarball directory %s", saveDir)
	}

	filename := fmt.Sprintf("%s-%s.tar.gz", "osqueryd", version)
	savePath := filepath.Join(saveDir, filename)
	level.Debug(n.logger).Log(
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

	source := filepath.Join(stagingPath, platform, "bin", "osqueryd")

	level.Debug(n.logger).Log(
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

	level.Info(n.logger).Log(
		"msg", n.desc,
		"output", savePath,
	)
	props.set(propOsqueryVersionTarballPath, savePath)
	level.Info(n.logger).Log(
		"msg", n.desc,
		"output", savePath,
	)
	return nil
}

func createOsqueryTaggedTarballs(n *node, props *properties) error {
	level.Debug(n.logger).Log(
		"msg", n.desc,
	)
	platform, err := props.getString(propPlatform)
	if err != nil {
		return err
	}
	version, err := props.getString(propOsqueryVersion)
	if err != nil {
		return err
	}
	channel, err := props.getString(propChannel)
	if err != nil {
		return err
	}
	saveDir := filepath.Join(stagingPath, mirrorBucketname, "kolide", "osqueryd", platform)
	filename := fmt.Sprintf("%s-%s.tar.gz", "osqueryd", version)
	versionTarball := filepath.Join(saveDir, filename)
	taggedFilename := fmt.Sprintf("%s-%s.tar.gz", "osqueryd", channel)
	taggedTarballPath := filepath.Join(saveDir, taggedFilename)
	level.Debug(n.logger).Log(
		"msg", "create tagged tarball",
		"output", taggedTarballPath,
	)
	if err := packaging.CopyFile(versionTarball, taggedTarballPath); err != nil {
		return errors.Wrapf(err, "copy file %s to %s", versionTarball, taggedTarballPath)
	}
	props.set(propOsqueryTaggedTarballPath, taggedTarballPath)
	level.Info(n.logger).Log(
		"msg", n.desc,
		"output", taggedTarballPath,
	)
	return nil
}

func uploadOsqueryToMirror(n *node, props *properties) error {
	level.Debug(n.logger).Log(
		"msg", n.desc,
	)
	platform, err := props.getString(propPlatform)
	if err != nil {
		return err
	}
	source, err := props.getString(propOsqueryVersionTarballPath)
	if err != nil {
		return err
	}
	if err := uploadToMirror(n.logger, platform, "osqueryd", source); err != nil {
		return err
	}
	level.Info(n.logger).Log(
		"msg", n.desc,
		"source", source,
	)
	source, err = props.getString(propOsqueryTaggedTarballPath)
	if err != nil {
		return err
	}
	if err := uploadToMirror(n.logger, platform, "osqueryd", source); err != nil {
		return err
	}
	level.Info(n.logger).Log(
		"msg", n.desc,
		"source", source,
	)
	return nil
}

func publishOsqueryToNotary(n *node, props *properties) error {
	level.Debug(n.logger).Log(
		"msg", n.desc,
	)
	platform, err := props.getString(propPlatform)
	if err != nil {
		return err
	}
	source, err := props.getString(propOsqueryVersionTarballPath)
	if err != nil {
		return err
	}
	if err := publishToNotary(n.logger, platform, "osqueryd", source); err != nil {
		return err
	}
	level.Info(n.logger).Log(
		"msg", n.desc,
		"source", source,
	)
	source, err = props.getString(propOsqueryTaggedTarballPath)
	if err != nil {
		return err
	}
	if err = publishToNotary(n.logger, platform, "osqueryd", source); err != nil {
		return nil
	}
	level.Info(n.logger).Log(
		"msg", n.desc,
		"source", source,
	)
	return nil
}

func createLauncherTarballs(n *node, props *properties) error {
	level.Debug(n.logger).Log(
		"msg", n.desc,
	)
	info := kv.Version()
	platform, err := props.getString(propPlatform)
	if err != nil {
		return err
	}
	tarball, err := createLauncherTarball(n.logger, platform, info.Version)
	if err != nil {
		return err
	}
	props.set(propLauncherVersionTarballPath, tarball)
	level.Info(n.logger).Log(
		"msg", n.desc,
		"source", tarball,
	)
	channel, err := props.getString(propChannel)
	if err != nil {
		return err
	}
	tarball, err = createLauncherTarball(n.logger, platform, channel)
	if err != nil {
		return err
	}
	props.set(propLauncherTaggedTarballPath, tarball)
	level.Info(n.logger).Log(
		"msg", n.desc,
		"source", tarball,
	)
	return nil
}

func uploadLauncherTarballs(n *node, props *properties) error {
	level.Debug(n.logger).Log(
		"msg", n.desc,
	)
	platform, err := props.getString(propPlatform)
	if err != nil {
		return err
	}
	tarball, err := props.getString(propLauncherVersionTarballPath)
	if err != nil {
		return err
	}
	if err = uploadToMirror(n.logger, platform, "launcher", tarball); err != nil {
		return err
	}
	level.Info(n.logger).Log(
		"msg", n.desc,
		"source", tarball,
	)
	tarball, err = props.getString(propLauncherTaggedTarballPath)
	if err != nil {
		return err
	}
	if err := uploadToMirror(n.logger, platform, "launcher", tarball); err != nil {
		return err
	}
	level.Info(n.logger).Log(
		"msg", n.desc,
		"source", tarball,
	)
	return nil
}

func publishLauncherToNotary(n *node, props *properties) error {
	level.Debug(n.logger).Log(
		"msg", n.desc,
	)
	platform, err := props.getString(propPlatform)
	if err != nil {
		return err
	}
	tarball, err := props.getString(propLauncherVersionTarballPath)
	if err != nil {
		return err
	}
	if err = publishToNotary(n.logger, platform, "launcher", tarball); err != nil {
		return err
	}
	level.Info(n.logger).Log(
		"msg", n.desc,
		"source", tarball,
	)
	tarball, err = props.getString(propLauncherTaggedTarballPath)
	if err != nil {
		return err
	}
	if err = publishToNotary(n.logger, platform, "launcher", tarball); err != nil {
		return err
	}
	level.Info(n.logger).Log(
		"msg", n.desc,
		"source", tarball,
	)
	return nil
}

func uploadToMirror(logger log.Logger, platform, binary, source string) error {
	level.Debug(logger).Log(
		"msg", "upload to mirror",
		"source", source,
		"msg", "starting",
	)
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return errors.Wrap(err, "preparing osquery mirror upload")
	}
	bkt := client.Bucket(mirrorBucketname)
	objectName := path.Join("kolide", binary, platform, filepath.Base(source))
	obj := bkt.Object(objectName)
	rdr, err := os.Open(source)
	if err != nil {
		return errors.Wrapf(err, "opening %s", source)
	}
	defer rdr.Close()

	level.Debug(logger).Log(
		"method", "update",
		"bucket", mirrorBucketname,
		"object", objectName,
	)
	wtr := obj.NewWriter(ctx)
	defer wtr.Close()
	_, err = io.Copy(wtr, rdr)
	if err != nil {
		return errors.Wrap(err, "writing to osquery mirror")
	}
	level.Debug(logger).Log(
		"method", "upload to mirror",
		"source", source,
		"msg", "complete",
	)
	return nil
}

func publishToNotary(logger log.Logger, platform, binary, archive string) error {
	target := path.Join(platform, filepath.Base(archive))
	gun := path.Join("kolide", binary)
	cmd := exec.Command(
		"notary",
		"add",
		gun,
		target,
		archive,
		"-p",
	)
	errNotary := func(err error, t, g string) error {
		return errors.Wrapf(err, "publishing target %s to gun %s", t, g)
	}
	// Notary writes error output to stdout.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return errNotary(err, target, gun)
	}
	if err := cmd.Start(); err != nil {
		return errNotary(err, target, gun)
	}
	var notaryTerminal bytes.Buffer
	if _, err := io.Copy(&notaryTerminal, stdout); err != nil {
		return errNotary(err, target, gun)
	}
	if err := cmd.Wait(); err != nil {
		level.Error(logger).Log(
			"msg", "notary failed",
			"err", err,
			"details", notaryTerminal.String(),
		)
		return errNotary(err, target, gun)
	}
	level.Info(logger).Log(
		"msg", "published target",
		"gun", gun,
		"target", target,
	)
	return nil
}

func createLauncherTarball(logger log.Logger, platform, versionOrChannel string) (string, error) {
	saveDir := filepath.Join(stagingPath, mirrorBucketname, "kolide", "launcher", platform)
	if err := os.MkdirAll(saveDir, packaging.DirMode); err != nil {
		return "", errors.Wrapf(err, "creating tarball dir %q", saveDir)
	}
	tarFilePath := filepath.Join(saveDir, fmt.Sprintf("launcher-%s.tar.gz", versionOrChannel))
	level.Debug(logger).Log(
		"msg", "creating tarball",
		"output", tarFilePath,
	)
	buildDir := filepath.Join(packaging.LauncherSource(), "build")
	source := filepath.Join(buildDir, platform, "launcher")

	fWtr, err := os.Create(tarFilePath)
	if err != nil {
		return "", errors.Wrapf(err, "creating tarball file %q", tarFilePath)
	}
	defer fWtr.Close()
	zipWtr := gzip.NewWriter(fWtr)
	defer zipWtr.Close()
	tarWtr := tar.NewWriter(zipWtr)
	defer tarWtr.Close()
	// add file to archive
	var (
		fRdr *os.File
		fi   os.FileInfo
		hdr  *tar.Header
	)
	if fRdr, err = os.Open(source); err != nil {
		return "", errors.Wrapf(err, "reading %q", source)
	}
	defer fRdr.Close()
	if fi, err = fRdr.Stat(); err != nil {
		return "", errors.Wrap(err, "could not get stat")
	}
	if hdr, err = tar.FileInfoHeader(fi, filepath.Base(source)); err != nil {
		return "", errors.Wrap(err, "getting file header")
	}
	if err = tarWtr.WriteHeader(hdr); err != nil {
		return "", errors.Wrap(err, "writing tar header")
	}
	if _, err = io.Copy(tarWtr, fRdr); err != nil {
		return "", errors.Wrap(err, "writing file to tar archive")
	}
	level.Info(logger).Log(
		"msg", "created archive",
		"source", source,
		"tar", tarFilePath,
	)
	return tarFilePath, nil
}

func extractLinux(logger log.Logger) error {
	savePath := filepath.Join(stagingPath, PlatformLinux, "bin", "osqueryd")
	if err := os.MkdirAll(filepath.Dir(savePath), packaging.DirMode); err != nil {
		return errors.Wrapf(err, "create directory %s", filepath.Dir(savePath))
	}
	out, err := os.OpenFile(savePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return errors.Wrapf(err, "create file %s", savePath)
	}
	defer out.Close()

	pkgPath := filepath.Join(stagingPath, PlatformLinux, "osquery.deb")

	level.Debug(logger).Log(
		"msg", "extracting osqueryd from deb archive",
		"package_path", pkgPath,
		"output_path", savePath,
	)

	debFile, closer, err := deb.LoadFile(pkgPath)
	if err != nil {
		return errors.Wrapf(err, "loading deb archive at %s", pkgPath)
	}
	defer closer()

	tr := debFile.Data

	found := false
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Wrap(err, "reading next header from osquery tarball")
		}
		if header.Name == "./usr/bin/osqueryd" {
			found = true
			level.Debug(logger).Log(
				"msg", "copying osqueryd from tar",
				"output_path", savePath,
				"file_size", header.Size,
			)
			if _, err := io.CopyN(out, tr, header.Size); err != nil {
				return errors.Wrapf(err, "copy %d bytes from tarball", header.Size)
			}
			break
		}
	}
	if !found {
		return fmt.Errorf("could not find osqueryd binary in deb package %s", pkgPath)
	}
	return nil
}

func extractDarwin(logger log.Logger) error {
	pkgPath := filepath.Join(stagingPath, PlatformDarwin, "osquery.pkg")
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
	level.Debug(logger).Log(
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

	if err := readCpioFile(logger, gzr, "./usr/local/bin/osqueryd", PlatformDarwin); err != nil {
		return err
	}

	return nil
}

// executes the cpio command to extract a file from the stream.
// using the exec package, because none of the Go libraries support
// the odc(070707) header yet.
func readCpioFile(logger log.Logger, input io.Reader, filename, platform string) error {
	tempdir, err := ioutil.TempDir("", "")
	if err != nil {
		return errors.Wrap(err, "create TempDir for cpio")
	}
	defer os.RemoveAll(tempdir)
	cmd := exec.Command("cpio", "-ivdm", filename)
	cmd.Dir = tempdir
	cmd.Stdin = input
	level.Debug(logger).Log(
		"msg", "executing cpio command",
		"cmd", strings.Join(cmd.Args, " "),
	)
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "run command %v", cmd.Args)
	}
	tmpbin := filepath.Join(tempdir, filepath.Clean(filename))
	newbin := filepath.Join(stagingPath, platform, "bin", filepath.Base(filename))
	if err := os.MkdirAll(filepath.Dir(newbin), packaging.DirMode); err != nil {
		return errors.Wrapf(err, "create directory %s", filepath.Dir(newbin))
	}
	level.Debug(logger).Log(
		"msg", "copying extracted file",
		"src", tmpbin,
		"dst", newbin,
	)
	return os.Rename(tmpbin, newbin)
}

func downloadOsqueryByPlatform(platform string) error {
	destination := filepath.Join(stagingPath, platform)
	if err := os.MkdirAll(destination, packaging.DirMode); err != nil {
		return errors.Wrapf(err, "creating folder for osquery downloads %q", destination)
	}
	var url string
	switch platform {
	case PlatformDarwin:
		url = osqueryDarwinSourceURL
		destination = filepath.Join(destination, "osquery.pkg")
	case PlatformLinux:
		url = osqueryLinuxSourceURL
		destination = filepath.Join(destination, "osquery.deb")
	case PlatformWindows:
		return ErrNotSupported
	default:
		return errors.Errorf("unknown platform %q", platform)
	}
	resp, err := http.Get(url)
	if err != nil {
		return errors.Wrapf(err, "downloading osquery package for %q from %q", platform, url)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("unexpected status %q downloading osquery package %q", resp.Status, platform)
	}
	out, err := os.OpenFile(destination, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return errors.Wrapf(err, "open or create file %s", destination)
	}
	defer out.Close()
	if _, err := io.Copy(out, resp.Body); err != nil {
		return errors.Wrap(err, "save downloaded osquery package to file")
	}
	return nil
}
