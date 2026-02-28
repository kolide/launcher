package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/packaging"
)

// runDownload downloads launcher or osqueryd from the TUF repo to the provided path.
// It's meant for use in CI pipelines and release verification.
//
// Usage: launcher download <launcher|osqueryd> [flags]
func runDownload(_ *multislogger.MultiSlogger, args []string) error {
	fs := flag.NewFlagSet("launcher download", flag.ExitOnError)

	var (
		flChannel  = fs.String("channel", "stable", "What channel to download from (or a specific version)")
		flDir      = fs.String("directory", ".", "Where to download the binary to")
		flPlatform = fs.String("platform", runtime.GOOS, "Target platform (darwin, linux, windows)")
		flArch     = fs.String("arch", runtime.GOARCH, "Target architecture (amd64, arm64)")
	)

	if err := fs.Parse(args); err != nil {
		return err
	}

	binary := fs.Arg(0)
	if binary == "" {
		return fmt.Errorf("must specify binary to download: launcher or osqueryd")
	}
	binary = strings.ToLower(binary)
	if binary != "launcher" && binary != "osqueryd" {
		return fmt.Errorf("binary must be launcher or osqueryd, got %q", binary)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*2)
	defer cancel()

	target := packaging.Target{}
	if err := target.PlatformFromString(*flPlatform); err != nil {
		return fmt.Errorf("error parsing platform: %w", err)
	}
	if *flPlatform == "darwin" {
		target.Arch = packaging.Universal
	} else if err := target.ArchFromString(*flArch); err != nil {
		return fmt.Errorf("error parsing arch: %w", err)
	}

	cacheDir, err := os.MkdirTemp("", binary+"-download")
	if err != nil {
		return fmt.Errorf("creating temp cache dir: %w", err)
	}
	defer os.RemoveAll(cacheDir)

	binaryName := target.PlatformBinaryName(binary)
	dlpath, err := packaging.FetchBinary(ctx, cacheDir, binary, binaryName, *flChannel, target)
	if err != nil {
		return fmt.Errorf("error fetching %s binary: %w", binary, err)
	}

	outfile := filepath.Join(*flDir, filepath.Base(dlpath))
	if err := fsutil.CopyFile(dlpath, outfile); err != nil {
		return fmt.Errorf("error copying %s binary: %w", binary, err)
	}

	fmt.Printf("Downloaded %s to: %s\n", binary, outfile)

	return nil
}
