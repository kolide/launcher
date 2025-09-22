package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/packaging"
)

// runDownloadOsquery downloads the stable osquery to the provided path. It's meant for use in out CI pipeline.
func runDownloadOsquery(_ *multislogger.MultiSlogger, args []string) error {
	fs := flag.NewFlagSet("launcher download-osquery", flag.ExitOnError)

	var (
		flChannel = fs.String("channel", "stable", "What channel to download from")
		flDir     = fs.String("directory", ".", "Where to download osquery to")
	)

	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*2)
	defer cancel()

	target := packaging.Target{}
	if err := target.PlatformFromString(runtime.GOOS); err != nil {
		return fmt.Errorf("error parsing platform: %w, %s", err, runtime.GOOS)
	}
	target.Arch = packaging.ArchFlavor(runtime.GOARCH)
	if runtime.GOOS == "darwin" {
		target.Arch = packaging.Universal
	}

	// We're reusing packaging code, which is based around having a persistent cache directory. It's not quite what
	// we want but it'll do
	cacheDir, err := os.MkdirTemp("", "osquery-download")
	if err != nil {
		return fmt.Errorf("creating temp cache dir: %w", err)
	}
	defer os.RemoveAll(cacheDir)

	dlpath, err := packaging.FetchBinary(ctx, cacheDir, "osqueryd", target.PlatformBinaryName("osqueryd"), *flChannel, target)
	if err != nil {
		return fmt.Errorf("error fetching binary osqueryd binary: %w", err)
	}

	outfile := filepath.Join(*flDir, filepath.Base(dlpath))
	if err := fsutil.CopyFile(dlpath, outfile); err != nil {
		return fmt.Errorf("error copying binary osqueryd binary: %w", err)
	}

	fmt.Printf("Downloaded to: %s\n", outfile)

	return nil
}
