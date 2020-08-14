package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/kolide/kit/logutil"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/updater/tuf"
	"github.com/pkg/errors"
)

// runUseVersion manually downloads the specified version, and
// installs it to local disk as if it were an automatic update.
func runUseVersion(args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts, extraArgs, err := parseOptions(args)
	if err != nil {
		return errors.Wrap(err, "parsing options")
	}

	// recreate the logger with  the appropriate level.
	logger := logutil.NewCLILogger(opts.Debug)
	ctx = ctxlog.NewContext(ctx, logger)

	if len(extraArgs) < 2 {
		return errors.New("Missing arguments")
	}

	var binaryPath string
	switch extraArgs[0] {
	case "launcher":
		binaryPath, err = os.Executable()
		if err != nil {
			return errors.Wrap(err, "finding launcher")
		}
	case "osqueryd":
		binaryPath = opts.OsquerydPath
	default:
		return errors.Errorf("Unknown binary name '%s'. Should be launcher or osqueryd", extraArgs[0])
	}

	requestedVersion := extraArgs[1]
	if requestedVersion == "" {
		return errors.New("Missing requested binary")
	}

	//castedVersion, ok := autoupdate.UpdateChannelrequestedVersion.(),)

	// This duplicates a bunch of the options parsing in
	// cmd/launcher/updater.go. But refactoring and merging them is fraught
	binaryUpdater, err := autoupdate.NewUpdater(
		binaryPath,
		opts.RootDirectory,
		autoupdate.WithLogger(logger),
		autoupdate.WithHTTPClient(httpClientFromOpts(opts)),
		autoupdate.WithNotaryURL(opts.NotaryServerURL),
		autoupdate.WithMirrorURL(opts.MirrorServerURL),
		autoupdate.WithNotaryPrefix(opts.NotaryPrefix),
		autoupdate.WithUpdateChannel(autoupdate.UpdateChannel(requestedVersion)),
	)
	if err != nil {
		return errors.Wrap(err, "create updater")
	}

	f, err := ioutil.TempFile("/tmp", "tuf-download")
	if err != nil {
		return errors.Wrap(err, "making tmpfile")
	}
	defer f.Close()

	if err := binaryUpdater.Download(f, tuf.WithFrequency(opts.AutoupdateInterval), tuf.WithLogger(logger)); err != nil {
		return errors.Wrap(err, "download")
	}

	fmt.Printf("\nDownloaded %s-%s to: %s\n", binaryPath, requestedVersion, f.Name())
	return nil

}
