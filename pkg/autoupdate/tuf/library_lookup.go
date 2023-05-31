package tuf

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/autoupdate"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type BinaryUpdateInfo struct {
	Path    string
	Version string
}

// CheckOutLatest returns the path to the latest downloaded executable for our binary, as well
// as its version.
func CheckOutLatest(ctx context.Context, binary autoupdatableBinary, rootDirectory string, updateDirectory string, channel string, logger log.Logger) (*BinaryUpdateInfo, error) {
	var span trace.Span
	ctx, span = otel.Tracer("launcher").Start(ctx, "CheckOutLatest")
	span.SetAttributes(attribute.String("binary", string(binary)))
	defer span.End()

	if updateDirectory == "" {
		updateDirectory = defaultLibraryDirectory(rootDirectory)
	}

	update, err := findExecutableFromRelease(ctx, binary, LocalTufDirectory(rootDirectory), channel, updateDirectory)
	if err == nil {
		return update, nil
	}

	level.Debug(logger).Log("msg", "could not find executable from release", "err", err)

	// If we can't find the specific release version that we should be on, then just return the executable
	// with the most recent version in the library
	return mostRecentVersion(ctx, binary, updateDirectory)
}

// findExecutableFromRelease looks at our local TUF repository to find the release for our
// given channel. If it's already downloaded, then we return its path and version.
func findExecutableFromRelease(ctx context.Context, binary autoupdatableBinary, tufRepositoryLocation string, channel string, baseUpdateDirectory string) (*BinaryUpdateInfo, error) {
	var span trace.Span
	ctx, span = otel.Tracer("launcher").Start(ctx, "findExecutableFromRelease")
	defer span.End()

	// Initialize a read-only TUF metadata client to parse the data we already have downloaded about releases.
	metadataClient, err := readOnlyTufMetadataClient(tufRepositoryLocation)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, errors.New("could not initialize TUF client, cannot find release")
	}

	// From already-downloaded metadata, look for the release version
	targets, err := metadataClient.Targets()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("could not get target: %w", err)
	}

	targetName, _, err := findRelease(ctx, binary, targets, channel)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("could not find release: %w", err)
	}

	targetPath, targetVersion := pathToTargetVersionExecutable(binary, targetName, baseUpdateDirectory)
	if autoupdate.CheckExecutable(ctx, targetPath, "--version") != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("version %s from target %s either not yet downloaded or corrupted: %w", targetVersion, targetName, err)
	}

	return &BinaryUpdateInfo{
		Path:    targetPath,
		Version: targetVersion,
	}, nil
}

// mostRecentVersion returns the path to the most recent, valid version available in the library for the
// given binary, along with its version.
func mostRecentVersion(ctx context.Context, binary autoupdatableBinary, baseUpdateDirectory string) (*BinaryUpdateInfo, error) {
	var span trace.Span
	ctx, span = otel.Tracer("launcher").Start(ctx, "mostRecentVersion")
	defer span.End()

	// Pull all available versions from library
	validVersionsInLibrary, _, err := sortedVersionsInLibrary(ctx, binary, baseUpdateDirectory)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("could not get sorted versions in library for %s: %w", binary, err)
	}

	// No valid versions in the library
	if len(validVersionsInLibrary) < 1 {
		return nil, errors.New("no versions in library")
	}

	// Versions are sorted in ascending order -- return the last one
	mostRecentVersionInLibraryRaw := validVersionsInLibrary[len(validVersionsInLibrary)-1]
	versionDir := filepath.Join(updatesDirectory(binary, baseUpdateDirectory), mostRecentVersionInLibraryRaw)
	return &BinaryUpdateInfo{
		Path:    executableLocation(versionDir, binary),
		Version: mostRecentVersionInLibraryRaw,
	}, nil
}
