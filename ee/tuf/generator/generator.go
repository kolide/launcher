package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/logutil"
	"github.com/theupdateframework/go-tuf/client"
	filejsonstore "github.com/theupdateframework/go-tuf/client/filejsonstore"
)

// initialRootJSON contains the trusted first version of root.json
// This is version 1 from https://tuf.kolide.com/repository/1.root.json

//go:embed assets/initial_root.json
var initialRootJSON []byte

func main() {
	logger := logutil.NewCLILogger(true)

	flagset := flag.NewFlagSet("tuf-generator", flag.ExitOnError)
	var (
		flDebug        = flagset.Bool("debug", false, "use a debug logger")
		flTufURL       = flagset.String("tuf-url", "https://tuf.kolide.com", "TUF repository URL")
		flMetadataPath = flagset.String("metadata-path", "/repository", "TUF metadata path")
		flOutputDirs   = flagset.String("output-dirs", "./assets/tuf,../../pkg/packaging/assets/tuf", "comma-separated output directories")
	)
	if err := flagset.Parse(os.Args[1:]); err != nil {
		level.Error(logger).Log("msg", "error parsing flags", "err", err)
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit outside of launcher proper
	}

	// relevel with the debug flag
	logger = logutil.NewCLILogger(*flDebug)

	// Validate required flags
	missingOpt := false
	for f, val := range map[string]string{
		"tuf-url":       *flTufURL,
		"metadata-path": *flMetadataPath,
		"output-dirs":   *flOutputDirs,
	} {
		if val == "" {
			level.Error(logger).Log("msg", "Missing required flag", "flag", f)
			missingOpt = true
		}
	}
	if missingOpt {
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit outside of launcher proper
	}

	ctx := context.Background()

	if err := updateTUFMetadata(ctx, logger, *flTufURL, *flMetadataPath, *flOutputDirs); err != nil {
		level.Error(logger).Log("msg", "error updating TUF metadata", "err", err)
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit outside of launcher proper
	}
}

func updateTUFMetadata(ctx context.Context, logger log.Logger, tufURL, metadataPath, outputDirsStr string) error {
	level.Info(logger).Log("msg", "Starting TUF metadata update", "url", tufURL)

	// Create a temporary directory to store TUF metadata
	tempDir, err := os.MkdirTemp("", "tuf-update")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	level.Debug(logger).Log("msg", "Created temporary directory", "path", tempDir)

	// Set up the local store
	localStore, err := filejsonstore.NewFileJSONStore(tempDir)
	if err != nil {
		return fmt.Errorf("initializing local TUF store: %w", err)
	}

	// Set up the remote store
	remoteOpts := client.HTTPRemoteOptions{
		MetadataPath: metadataPath,
	}

	level.Debug(logger).Log(
		"msg", "Configuring remote TUF store",
		"url", tufURL,
		"metadata_path", metadataPath,
	)

	remoteStore, err := client.HTTPRemoteStore(tufURL, &remoteOpts, http.DefaultClient)
	if err != nil {
		return fmt.Errorf("initializing remote TUF store: %w", err)
	}

	// Initialize the TUF client with our initial root
	tufClient := client.NewClient(localStore, remoteStore)
	level.Debug(logger).Log("msg", "Initializing TUF client with initial root")

	if err := tufClient.Init(initialRootJSON); err != nil {
		return fmt.Errorf("initializing TUF client: %w", err)
	}

	// Update the root metadata to the latest version
	level.Info(logger).Log("msg", "Updating TUF metadata to latest version")

	_, err = tufClient.Update()
	if err != nil {
		return fmt.Errorf("updating TUF metadata: %w", err)
	}

	// Get the latest root.json
	metadata, err := localStore.GetMeta()
	if err != nil {
		return fmt.Errorf("reading updated metadata from local store: %w", err)
	}
	latestRoot := metadata["root.json"]

	level.Debug(logger).Log("msg", "Read updated root.json", "size_bytes", len(latestRoot))

	// Parse and process output directories
	outputDirs := strings.Split(outputDirsStr, ",")
	if len(outputDirs) == 0 {
		return errors.New("no output directories specified")
	}

	// Write the updated root.json to all specified locations
	for _, dir := range outputDirs {
		// Clean the path to handle any whitespace or other issues
		dir = strings.TrimSpace(dir)
		path := filepath.Join(dir, "root.json")

		// Ensure directory exists
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating output directory %s: %w", filepath.Dir(path), err)
		}

		level.Debug(logger).Log("msg", "Writing updated root.json", "path", path)

		if err := os.WriteFile(path, latestRoot, 0644); err != nil {
			return fmt.Errorf("writing updated root.json to %s: %w", path, err)
		}

		level.Info(logger).Log("msg", "Successfully updated root.json", "path", path)
	}

	level.Info(logger).Log("msg", "TUF metadata update completed successfully")
	return nil
}
