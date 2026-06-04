package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kolide/launcher/v2/ee/tuf/simpleclient"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
)

// runDownload downloads launcher or osqueryd from the TUF repo with TUF verification
// and extracts the tarball contents to the output directory.
func runDownload(slogger *multislogger.MultiSlogger, args []string) error {
	fs := flag.NewFlagSet("launcher download", flag.ExitOnError)

	var (
		flBinary   = fs.String("binary", "", "Binary to download, e.g. launcher or osqueryd (omit to list available binaries)")
		flChannel  = fs.String("channel", "stable", "What channel to download from (or a specific version)")
		flDir      = fs.String("directory", ".", "Output directory (unless -raw is set, a subdirectory named after the binary will be created)")
		flPlatform = fs.String("platform", runtime.GOOS, "Target platform (darwin, linux, windows)")
		flArch     = fs.String("arch", runtime.GOARCH, "Target architecture (amd64, arm64; darwin always uses universal)")
		flRaw      = fs.Bool("raw", false, "Write verified tarball as-is instead of extracting")
		flDebug    = fs.Bool("debug", false, "Enable debug logging")
	)

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	level := slog.LevelInfo
	if *flDebug {
		level = slog.LevelDebug
	}
	slogger.AddHandler(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	binary := strings.ToLower(*flBinary)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*2)
	defer cancel()

	if binary == "" {
		binaries, err := simpleclient.ListTargets(ctx, slogger.Logger, nil)
		if err != nil {
			return fmt.Errorf("listing binaries: %w", err)
		}
		for _, b := range binaries {
			fmt.Println(b)
		}
		return nil
	}

	tarGzBytes, err := simpleclient.Download(ctx, slogger.Logger, binary, *flPlatform, *flArch, *flChannel, nil)
	if err != nil {
		return fmt.Errorf("error fetching %s: %w", binary, err)
	}

	if *flRaw {
		if err := os.MkdirAll(*flDir, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", *flDir, err)
		}
		outFile := filepath.Join(*flDir, binary+".tar.gz")
		if err := os.WriteFile(outFile, tarGzBytes, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", outFile, err)
		}
		fmt.Println(outFile)
		return nil
	}

	destDir := filepath.Join(*flDir, binary)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", destDir, err)
	}

	if err := extractTarGz(bytes.NewReader(tarGzBytes), destDir); err != nil {
		return fmt.Errorf("error extracting: %w", err)
	}

	fmt.Printf("Downloaded and extracted %s to: %s\n", binary, destDir)

	return nil
}

func extractTarGz(r io.Reader, destDir string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("creating gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		clean := filepath.Clean(header.Name)
		if strings.Contains(clean, "..") {
			return fmt.Errorf("tar entry %q contains path traversal", header.Name)
		}
		target := filepath.Join(destDir, clean)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("creating directory %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("creating parent directory for %s: %w", target, err)
			}
			if err := extractFile(target, header.Mode, tr); err != nil {
				return fmt.Errorf("extracting file %s: %w", target, err)
			}
		}
	}

	return nil
}

func extractFile(dest string, mode int64, r io.Reader) (retErr error) {
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(mode))
	if err != nil {
		return fmt.Errorf("creating file %s: %w", dest, err)
	}
	defer func() {
		if err := f.Close(); retErr == nil && err != nil {
			retErr = fmt.Errorf("closing file %s: %w", dest, err)
		}
	}()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("writing file %s: %w", dest, err)
	}

	return nil
}
