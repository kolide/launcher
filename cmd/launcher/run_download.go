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

	"github.com/kolide/launcher/ee/tuf/simpleclient"
	"github.com/kolide/launcher/pkg/log/multislogger"
)

// runDownload downloads launcher or osqueryd from the TUF repo with TUF verification.
// It's meant for use in CI pipelines and release verification.
//
// Usage: launcher download [flags]
func runDownload(_ *multislogger.MultiSlogger, args []string) error {
	fs := flag.NewFlagSet("launcher download", flag.ExitOnError)

	var (
		flBinary   = fs.String("binary", "", "Binary to download: launcher or osqueryd")
		flChannel  = fs.String("channel", "stable", "What channel to download from (or a specific version)")
		flDir      = fs.String("directory", ".", "Where to download the binary to")
		flPlatform = fs.String("platform", runtime.GOOS, "Target platform (darwin, linux, windows)")
		flArch     = fs.String("arch", runtime.GOARCH, "Target architecture (amd64, arm64)")
	)

	if err := fs.Parse(args); err != nil {
		return err
	}

	binary := strings.ToLower(*flBinary)
	if binary == "" {
		return fmt.Errorf("must specify --binary (launcher or osqueryd)")
	}
	if binary != "launcher" && binary != "osqueryd" {
		return fmt.Errorf("binary must be launcher or osqueryd, got %q", binary)
	}

	binaryName := binary
	if *flPlatform == "windows" {
		binaryName += ".exe"
	}
	outfile := filepath.Join(*flDir, binaryName)

	if err := os.MkdirAll(*flDir, 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	f, err := os.OpenFile(outfile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("opening output file: %w", err)
	}
	defer f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*2)
	defer cancel()

	if err := simpleclient.Download(ctx, binary, *flPlatform, *flArch, *flChannel, f, nil); err != nil {
		return fmt.Errorf("error fetching %s binary: %w", binary, err)
	}

	fmt.Printf("Downloaded %s to: %s\n", binary, outfile)

	return nil
}
