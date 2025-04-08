package tuf

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/theupdateframework/go-tuf/data"
	tufutil "github.com/theupdateframework/go-tuf/util"
)

// updateLibraryManager manages the update libraries for launcher and osquery.
// It downloads and verifies new updates, and moves them to the appropriate
// location in the library specified by the version associated with that update.
// It also ensures that old updates are removed when they are no longer needed.
type updateLibraryManager struct {
	mirrorUrl    string // dl.kolide.co
	mirrorClient *http.Client
	baseDir      string
	lock         *libraryLock
	slogger      *slog.Logger
}

func newUpdateLibraryManager(mirrorUrl string, mirrorClient *http.Client, baseDir string, slogger *slog.Logger) (*updateLibraryManager, error) {
	ulm := updateLibraryManager{
		mirrorUrl:    mirrorUrl,
		mirrorClient: mirrorClient,
		baseDir:      baseDir,
		lock:         newLibraryLock(),
		slogger:      slogger.With("component", "tuf_autoupdater_library_manager"),
	}

	// Ensure the updates directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("could not make base directory for updates library: %w", err)
	}

	// Create the update library
	for _, binary := range binaries {
		if err := os.MkdirAll(updatesDirectory(binary, baseDir), 0755); err != nil {
			return nil, fmt.Errorf("could not make updates directory for %s: %w", binary, err)
		}
	}

	return &ulm, nil
}

// updatesDirectory returns the update library location for the given binary.
func updatesDirectory(binary autoupdatableBinary, baseUpdateDirectory string) string {
	return filepath.Join(baseUpdateDirectory, string(binary))
}

// Available determines if the given target is already available in the update library.
func (ulm *updateLibraryManager) Available(binary autoupdatableBinary, targetFilename string) bool {
	executablePath, _ := pathToTargetVersionExecutable(binary, targetFilename, ulm.baseDir)
	// First, check if the file even exists, before we do a full executable check
	if _, err := os.Stat(executablePath); err != nil && errors.Is(err, os.ErrNotExist) {
		return false
	}
	return CheckExecutable(context.TODO(), ulm.slogger, executablePath, "--version") == nil
}

// pathToTargetVersionExecutable returns the path to the executable for the desired target,
// along with its version.
func pathToTargetVersionExecutable(binary autoupdatableBinary, targetFilename string, baseUpdateDirectory string) (string, string) {
	targetVersion := versionFromTarget(binary, targetFilename)
	versionDir := filepath.Join(updatesDirectory(binary, baseUpdateDirectory), targetVersion)
	return executableLocation(versionDir, binary), targetVersion
}

// AddToLibrary adds the given target file to the library for the given binary,
// downloading and verifying it if it's not already there.
func (ulm *updateLibraryManager) AddToLibrary(binary autoupdatableBinary, currentVersion string, targetFilename string, targetMetadata data.TargetFileMeta) error {
	// Acquire lock for modifying the library
	ulm.lock.Lock(binary)
	defer ulm.lock.Unlock(binary)

	if currentVersion == versionFromTarget(binary, targetFilename) {
		return nil
	}

	if ulm.Available(binary, targetFilename) {
		return nil
	}

	stagedUpdatePath, err := ulm.stageAndVerifyUpdate(binary, targetFilename, targetMetadata)
	// Remove downloaded archives after update, regardless of success -- this will run before the unlock
	defer func() {
		if stagedUpdatePath == "" {
			return
		}
		dirToRemove := filepath.Dir(stagedUpdatePath)
		if err := backoff.WaitFor(func() error {
			return os.RemoveAll(dirToRemove) //revive:disable-line -- incorrectly flags `defer: return in a defer function has no effect`
		}, 500*time.Millisecond, 100*time.Millisecond); err != nil {
			ulm.slogger.Log(context.TODO(), slog.LevelWarn,
				"could not remove temp staging directory",
				"directory", dirToRemove,
				"err", err,
			)
		}
	}()
	if err != nil {
		return fmt.Errorf("could not stage update: %w", err)
	}

	if err := ulm.moveVerifiedUpdate(binary, targetFilename, stagedUpdatePath); err != nil {
		return fmt.Errorf("could not move verified update: %w", err)
	}

	return nil
}

// stageAndVerifyUpdate downloads the update indicated by `targetFilename` and verifies it against
// the given, validated local metadata.
func (ulm *updateLibraryManager) stageAndVerifyUpdate(binary autoupdatableBinary, targetFilename string, localTargetMetadata data.TargetFileMeta) (string, error) {
	stagingDir, err := ulm.tempDir(binary, fmt.Sprintf("staged-updates-%s", versionFromTarget(binary, targetFilename)))
	if err != nil {
		return "", fmt.Errorf("could not create temporary directory for downloading target: %w", err)
	}
	stagedUpdatePath := filepath.Join(stagingDir, targetFilename)

	// Ensure we set a timeout on our request to download the binary
	timeout := ulm.mirrorClient.Timeout
	if timeout == 0 {
		// Set a high-but-reasonable default
		timeout = 8 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Request download from mirror
	downloadPath := path.Join("/", "kolide", string(binary), runtime.GOOS, PlatformArch(), targetFilename)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ulm.mirrorUrl+downloadPath, nil)
	if err != nil {
		return stagedUpdatePath, fmt.Errorf("creating request to download target %s: %w", targetFilename, err)
	}
	resp, err := ulm.mirrorClient.Do(req)
	if err != nil {
		return stagedUpdatePath, fmt.Errorf("could not make request to download target %s: %w", targetFilename, err)
	}
	defer resp.Body.Close()

	// Wrap the download in a LimitReader so we read at most localMeta.Length bytes
	stream := io.LimitReader(resp.Body, localTargetMetadata.Length)
	var fileBuffer bytes.Buffer

	// Read the target file, simultaneously writing it to our file buffer and generating its metadata
	actualTargetMeta, err := tufutil.GenerateTargetFileMeta(io.TeeReader(stream, io.Writer(&fileBuffer)), localTargetMetadata.HashAlgorithms()...)
	if err != nil {
		return stagedUpdatePath, fmt.Errorf("could not write downloaded target %s to file %s and compute its metadata: %w", targetFilename, stagedUpdatePath, err)
	}

	// Verify the actual download against the confirmed local metadata
	if err := tufutil.TargetFileMetaEqual(actualTargetMeta, localTargetMetadata); err != nil {
		return stagedUpdatePath, fmt.Errorf("verification failed for target %s staged at %s: %w", targetFilename, stagedUpdatePath, err)
	}

	// Everything looks good: create the file and write it to disk.
	// We create the file with 0655 permissions to prevent any other user from writing to this file
	// before we can copy to it.
	out, err := os.OpenFile(stagedUpdatePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0655)
	if err != nil {
		return "", fmt.Errorf("could not create file at %s: %w", stagedUpdatePath, err)
	}
	if _, err := io.Copy(out, &fileBuffer); err != nil {
		if err := out.Close(); err != nil {
			return stagedUpdatePath, fmt.Errorf("could not write downloaded target %s to file %s and could not close file: %w", targetFilename, stagedUpdatePath, err)
		}
		return stagedUpdatePath, fmt.Errorf("could not write downloaded target %s to file %s: %w", targetFilename, stagedUpdatePath, err)
	}
	if err := out.Close(); err != nil {
		return stagedUpdatePath, fmt.Errorf("could not close downloaded target file %s after writing: %w", targetFilename, err)
	}

	return stagedUpdatePath, nil
}

// tempDir creates a directory inside of the updates directory. It is the caller's responsibility to remove
// the directory when it is no longer needed.
func (ulm *updateLibraryManager) tempDir(binary autoupdatableBinary, pattern string) (string, error) {
	directory := filepath.Join(updatesDirectory(binary, ulm.baseDir), fmt.Sprintf("%s-%d", pattern, time.Now().UnixMicro()))
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", fmt.Errorf("making dir %s: %w", directory, err)
	}

	return directory, nil
}

// moveVerifiedUpdate untars the update and performs final checks to make sure that it's a valid, working update.
// Finally, it moves the update to its correct versioned location in the update library for the given binary.
func (ulm *updateLibraryManager) moveVerifiedUpdate(binary autoupdatableBinary, targetFilename string, stagedUpdate string) error {
	targetVersion := versionFromTarget(binary, targetFilename)
	stagedVersionedDirectory, err := ulm.tempDir(binary, targetVersion)
	if err != nil {
		return fmt.Errorf("could not create temporary directory for untarring and validating new update: %w", err)
	}
	defer func() {
		// In case of error, clean up the staged version and its directory
		if _, err := os.Stat(stagedVersionedDirectory); err == nil || !os.IsNotExist(err) {
			if err := backoff.WaitFor(func() error {
				return os.RemoveAll(stagedVersionedDirectory) //revive:disable-line -- incorrectly flags `defer: return in a defer function has no effect`
			}, 500*time.Millisecond, 100*time.Millisecond); err != nil {
				ulm.slogger.Log(context.TODO(), slog.LevelWarn,
					"could not remove staged update",
					"directory", stagedVersionedDirectory,
					"err", err,
				)
			}
		}
	}()

	// Untar the archive.
	if err := untar(stagedVersionedDirectory, stagedUpdate); err != nil {
		return fmt.Errorf("could not untar update to %s: %w", stagedVersionedDirectory, err)
	}

	// Make sure that the binary is executable
	if err := os.Chmod(executableLocation(stagedVersionedDirectory, binary), 0755); err != nil {
		return fmt.Errorf("could not set +x permissions on executable: %w", err)
	}

	// If necessary, patch the executable (NixOS only)
	if err := patchExecutable(executableLocation(stagedVersionedDirectory, binary)); err != nil {
		return fmt.Errorf("could not patch executable: %w", err)
	}

	// Validate the executable -- the executable check will occasionally time out, especially on Windows,
	// and we aren't in a rush here, so we retry a couple times.
	if err := backoff.WaitFor(func() error {
		return CheckExecutable(context.TODO(), ulm.slogger, executableLocation(stagedVersionedDirectory, binary), "--version")
	}, 45*time.Second, 15*time.Second); err != nil {
		return fmt.Errorf("could not verify executable after retries: %w", err)
	}

	// All good! Shelve it in the library under its version. We also perform some retries
	// here for Windows, since sometimes Windows will think the binary is still in use and
	// will refuse to move it.
	newUpdateDirectory := filepath.Join(updatesDirectory(binary, ulm.baseDir), targetVersion)
	if err := backoff.WaitFor(func() error {
		return os.Rename(stagedVersionedDirectory, newUpdateDirectory)
	}, 6*time.Second, 2*time.Second); err != nil {
		return fmt.Errorf("could not move staged target %s from %s to %s after retries: %w", targetFilename, stagedVersionedDirectory, newUpdateDirectory, err)
	}

	// Need rwxr-xr-x so that the desktop (running as user) can execute the downloaded binary too
	if err := os.Chmod(newUpdateDirectory, 0755); err != nil {
		return fmt.Errorf("could not chmod %s: %w", newUpdateDirectory, err)
	}

	return nil
}

// untar extracts the archive `source` to the given `destinationDir`. It sanitizes
// extract paths and file permissions.
func untar(destinationDir string, source string) error {
	f, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("opening source: %w", err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("creating gzip reader from %s: %w", source, err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar file: %w", err)
		}

		if err := sanitizeExtractPath(destinationDir, header.Name); err != nil {
			return fmt.Errorf("checking filename: %w", err)
		}

		destPath := filepath.Join(destinationDir, header.Name)
		info := header.FileInfo()
		if info.IsDir() {
			if err = os.MkdirAll(destPath, sanitizePermissions(info)); err != nil {
				return fmt.Errorf("creating directory %s for tar file: %w", destPath, err)
			}
			continue
		}

		if err := writeBundleFile(destPath, sanitizePermissions(info), tr); err != nil {
			return fmt.Errorf("writing file: %w", err)
		}
	}
	return nil
}

// sanitizeExtractPath checks that the supplied extraction path is not
// vulnerable to zip slip attacks. See https://snyk.io/research/zip-slip-vulnerability
func sanitizeExtractPath(filePath string, destination string) error {
	destpath := filepath.Join(destination, filePath)
	if !strings.HasPrefix(destpath, filepath.Clean(destination)+string(os.PathSeparator)) {
		return fmt.Errorf("illegal file path %s", filePath)
	}
	return nil
}

// Allow owner read/write/execute, and only read/execute for all others.
const allowedPermissionBits fs.FileMode = fs.ModeType | 0755

// sanitizePermissions ensures that only the file owner has write permissions.
func sanitizePermissions(fileInfo fs.FileInfo) fs.FileMode {
	return fileInfo.Mode() & allowedPermissionBits
}

// writeBundleFile reads from the given reader to create a file at the given path, with the desired permissions.
func writeBundleFile(destPath string, perm fs.FileMode, srcReader io.Reader) error {
	file, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return fmt.Errorf("opening %s: %w", destPath, err)
	}
	if _, err := io.Copy(file, srcReader); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			return fmt.Errorf("copying to %s: %v; close error: %w", destPath, err, closeErr)
		}
		return fmt.Errorf("copying to %s: %w", destPath, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", destPath, err)
	}
	return nil
}

// removeUpdate removes a given version from the given binary's update library.
func (ulm *updateLibraryManager) removeUpdate(binary autoupdatableBinary, binaryVersion string) {
	directoryToRemove := filepath.Join(updatesDirectory(binary, ulm.baseDir), binaryVersion)
	if err := os.RemoveAll(directoryToRemove); err != nil {
		ulm.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not remove update",
			"directory", directoryToRemove,
			"err", err,
		)
	} else {
		ulm.slogger.Log(context.TODO(), slog.LevelDebug,
			"removed update",
			"directory", directoryToRemove,
		)
	}
}

// TidyLibrary reviews all updates in the library for the binary and removes any old versions
// that are no longer needed. It will always preserve the current running binary, and then the
// two most recent valid versions. It will remove versions it cannot validate.
func (ulm *updateLibraryManager) TidyLibrary(binary autoupdatableBinary, currentVersion string) {
	// Acquire lock for modifying the library
	ulm.lock.Lock(binary)
	defer ulm.lock.Unlock(binary)

	// Remove any updates we no longer need
	if currentVersion == "" {
		ulm.slogger.Log(context.TODO(), slog.LevelWarn,
			"cannot tidy update library without knowing current running version",
			"binary", binary,
		)
		return
	}

	const numberOfVersionsToKeep = 2

	versionsInLibrary, invalidVersionsInLibrary, err := sortedVersionsInLibrary(context.Background(), ulm.slogger, binary, ulm.baseDir)
	if err != nil {
		ulm.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not get versions in library to tidy update library",
			"binary", binary,
			"err", err,
		)
		return
	}

	for _, invalidVersion := range invalidVersionsInLibrary {
		ulm.slogger.Log(context.TODO(), slog.LevelWarn,
			"updates library contains invalid version",
			"library_path", invalidVersion,
			"binary", binary,
			"err", err,
		)
		ulm.removeUpdate(binary, invalidVersion)
	}

	if len(versionsInLibrary) <= numberOfVersionsToKeep {
		ulm.slogger.Log(context.TODO(), slog.LevelInfo,
			"no need to tidy library",
			"binary", binary,
		)
		return
	}

	// Loop through, looking at the most recent versions first, and remove all once we hit nonCurrentlyRunningVersionsKept valid executables
	nonCurrentlyRunningVersionsKept := 0
	for i := len(versionsInLibrary) - 1; i >= 0; i -= 1 {
		// Always keep the current running executable
		if versionsInLibrary[i] == currentVersion {
			continue
		}

		// If we've already hit the number of versions to keep, then start to remove the older ones.
		// We want to keep numberOfVersionsToKeep total, saving a spot for the currently running version.
		if nonCurrentlyRunningVersionsKept >= numberOfVersionsToKeep-1 {
			ulm.removeUpdate(binary, versionsInLibrary[i])
			continue
		}

		nonCurrentlyRunningVersionsKept += 1
	}

	ulm.slogger.Log(context.TODO(), slog.LevelInfo,
		"tidied update library",
		"binary", binary,
	)
}

// sortedVersionsInLibrary looks through the update library for the given binary to validate and sort all
// available versions. It returns a sorted list of the valid versions, a list of invalid versions, and
// an error only when unable to glob for versions.
func sortedVersionsInLibrary(ctx context.Context, slogger *slog.Logger, binary autoupdatableBinary, baseUpdateDirectory string) ([]string, []string, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	rawVersionsInLibrary, err := filepath.Glob(filepath.Join(updatesDirectory(binary, baseUpdateDirectory), "*"))
	if err != nil {
		return nil, nil, fmt.Errorf("could not glob for updates in library: %w", err)
	}

	versionsInLibrary := make([]*semver.Version, 0)
	invalidVersions := make([]string, 0)
	for _, rawVersionWithPath := range rawVersionsInLibrary {
		rawVersion := filepath.Base(rawVersionWithPath)
		v, err := semver.NewVersion(rawVersion)
		if err != nil {
			slogger.Log(ctx, slog.LevelWarn,
				"detected invalid binary version while parsing raw version",
				"raw_version", rawVersion,
				"binary", binary,
				"err", err,
			)

			invalidVersions = append(invalidVersions, rawVersion)
			continue
		}

		versionDir := filepath.Join(updatesDirectory(binary, baseUpdateDirectory), rawVersion)
		if err := CheckExecutable(ctx, slogger, executableLocation(versionDir, binary), "--version"); err != nil {
			traces.SetError(span, err)
			slogger.Log(ctx, slog.LevelWarn,
				"detected invalid binary version while checking executable",
				"version_dir", versionDir,
				"binary", binary,
				"err", err,
			)

			invalidVersions = append(invalidVersions, rawVersion)
			continue
		}

		// We have to swap the hyphen in the prerelease for a period (30-abcdabcd to 30.abcdabcd) so that the
		// semver library can correctly compare prerelease values.
		if v.Prerelease() != "" {
			versionWithUpdatedPrerelease, err := v.SetPrerelease(strings.Replace(v.Prerelease(), "-", ".", -1))
			if err == nil {
				v = &versionWithUpdatedPrerelease
			}
		}

		versionsInLibrary = append(versionsInLibrary, v)
	}

	// Sort the versions (ascending order)
	sort.Sort(semver.Collection(versionsInLibrary))

	// Transform versions back into strings now that we've finished sorting them; swap the prerelease value back.
	versionsInLibraryStr := make([]string, len(versionsInLibrary))
	for i, v := range versionsInLibrary {
		if v.Prerelease() != "" {
			versionWithUpdatedPrerelease, err := v.SetPrerelease(strings.Replace(v.Prerelease(), ".", "-", -1))
			if err == nil {
				v = &versionWithUpdatedPrerelease
			}
		}
		versionsInLibraryStr[i] = v.Original()
	}

	return versionsInLibraryStr, invalidVersions, nil
}
