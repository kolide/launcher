package autoupdate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
)

// This suffix is added to the binary path to find the updates
const updateDirSuffix = "-updates"

type newestSettings struct {
	deleteOld bool
}

type newestOption func(*newestSettings)

func DeleteOldUpdates() newestOption {
	return func(no *newestSettings) {
		no.deleteOld = true
	}
}

// FindNewestSelf invokes `FindNewest` with the running binary path,
// as determined by os.Executable. However, if the current running
// version is the same as the newest on disk, it will return empty string.
func FindNewestSelf(ctx context.Context, opts ...newestOption) (string, error) {
	logger := log.With(ctxlog.FromContext(ctx), "caller", "autoupdate.FindNewestSelf")

	exPath, err := os.Executable()
	if err != nil {
		return "", errors.Wrap(err, "determine running executable path")
	}

	if exPath == "" {
		return "", errors.New("can't find newest empty string")
	}

	newest := FindNewest(ctx, exPath, opts...)

	if newest == "" {
		return "", nil
	}

	if exPath == newest {
		return "", nil
	}

	level.Debug(logger).Log(
		"msg", "found an update",
		"newest", newest,
		"exPath", exPath,
	)

	return newest, nil
}

// FindNewest takes the full path to a binary, and returns the newest
// update on disk. If there are no updates on disk, it returns the
// original path. It will return the same fullBinaryPath if that is
// the newest version.
func FindNewest(ctx context.Context, fullBinaryPath string, opts ...newestOption) string {
	logger := log.With(ctxlog.FromContext(ctx), "caller", "autoupdate.FindNewest")

	if fullBinaryPath == "" {
		level.Debug(logger).Log("msg", "called with empty string")
		return ""
	}

	logger = log.With(logger, "fullBinaryPath", fullBinaryPath)

	newestSettings := &newestSettings{}
	for _, opt := range opts {
		opt(newestSettings)
	}

	updateDir := getUpdateDir(fullBinaryPath)
	binaryName := filepath.Base(fullBinaryPath)

	fileGlob := filepath.Join(updateDir, "*", binaryName)

	possibleUpdates, err := filepath.Glob(fileGlob)
	if err != nil {
		level.Error(logger).Log("msg", "globbing for updates", "err", err)
		return fullBinaryPath
	}

	sort.Strings(possibleUpdates)

	// iterate backwards over files, looking for a suitable binary
	foundFile := ""
	for i := len(possibleUpdates) - 1; i >= 0; i-- {
		file := possibleUpdates[i]

		if foundFile != "" {
			if !newestSettings.deleteOld {
				continue
			}

			basedir := filepath.Dir(file)
			spew.Dump(basedir)
			level.Debug(logger).Log("msg", "deleting old updates", "dir", basedir)
			if err := os.RemoveAll(basedir); err != nil {
				spew.Dump(err)
				level.Error(logger).Log("msg", "error deleting old update dir", "dir", basedir, "err", err)
			}
		}

		if err := checkExecutable(file); err != nil {
			level.Error(logger).Log("msg", "not executable!!", "binary", file, "reason", err)
			continue
		}

		foundFile = file
	}

	if foundFile != "" {
		return foundFile
	}

	level.Info(logger).Log("msg", "no updates found")

	if err := checkExecutable(fullBinaryPath); err == nil {
		return fullBinaryPath
	}

	level.Debug(logger).Log("msg", "fullBinaryPath not executable. Returning nil")
	return ""
}

// getUpdateDir returns the expected update path for a given
// binary. It should work when called with either a base executable
// `/usr/local/bin/launcher` or with an existing updated
// `/usr/local/bin/launcher-updates/1234/launcher`.
//
// It makes some string assumptions about how things are named.
func getUpdateDir(fullBinaryPath string) string {
	// These are cases that shouldn't really happen. But, this is
	// a bare string function. So return "" when they do.
	if strings.HasSuffix(fullBinaryPath, "/") {
		fullBinaryPath = strings.TrimSuffix(fullBinaryPath, "/")
	}

	if fullBinaryPath == "" {
		return ""
	}

	// If we SplitN on updateDirSuffix, we will either get an
	// array, or the full string back. This means we can forgo a
	// strings.Contains, and just use the returned element
	components := strings.SplitN(fullBinaryPath, updateDirSuffix, 2)

	return fmt.Sprintf("%s%s", components[0], updateDirSuffix)
}

// FindBaseDir takes a binary path, that may or may not include the
// update directory, and returns the base directory. It's used by the
// launcher runtime in finding the various binaries.
func FindBaseDir(path string) string {
	if path == "" {
		return ""
	}

	components := strings.SplitN(path, updateDirSuffix, 2)
	return filepath.Dir(components[0])
}
