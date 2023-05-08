package tufci

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/theupdateframework/go-tuf"
)

// InitRemoteTufServer sets up a local TUF repo with some targets to serve metadata about; returns the URL
// of a test HTTP server to serve that metadata and the root JSON needed to initialize a client.
func InitRemoteTufServer(t *testing.T, testReleaseVersion string) (tufServerURL string, rootJson []byte) {
	tufDir := t.TempDir()

	// Initialize repo with store
	localStore := tuf.FileSystemStore(tufDir, nil)
	repo, err := tuf.NewRepo(localStore)
	require.NoError(t, err, "could not create new tuf repo")
	require.NoError(t, repo.Init(false), "could not init new tuf repo")

	// Gen keys
	_, err = repo.GenKey("root")
	require.NoError(t, err, "could not gen root key")
	_, err = repo.GenKey("targets")
	require.NoError(t, err, "could not gen targets key")
	_, err = repo.GenKey("snapshot")
	require.NoError(t, err, "could not gen snapshot key")
	_, err = repo.GenKey("timestamp")
	require.NoError(t, err, "could not gen timestamp key")

	// Sign the root metadata file
	require.NoError(t, repo.Sign("root.json"), "could not sign root metadata file")

	// Create test binaries and release files per binary and per release channel
	for _, b := range []string{"osqueryd", "launcher"} {
		for _, v := range []string{"0.1.1", "0.12.3-deadbeef", testReleaseVersion} {
			binaryFileName := fmt.Sprintf("%s-%s.tar.gz", b, v)

			// Create a valid test binary -- an archive of an executable with the proper directory structure
			// that will actually run -- if this is the release version we care about. If this is not the
			// release version we care about, then just create a small text file since it won't be downloaded
			// and evaluated.
			if v == testReleaseVersion {
				// Create test binary and copy it to the staged targets directory
				stagedTargetsDir := filepath.Join(tufDir, "staged", "targets", b, runtime.GOOS)
				executablePath := executableLocation(stagedTargetsDir, b)
				require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0777), "could not make staging directory")
				CopyBinary(t, executablePath)
				require.NoError(t, os.Chmod(executablePath, 0755))

				// Compress the binary or app bundle
				compress(t, binaryFileName, stagedTargetsDir, stagedTargetsDir, b)
			} else {
				// Create and commit a test binary
				require.NoError(t, os.MkdirAll(filepath.Join(tufDir, "staged", "targets", b, runtime.GOOS), 0777), "could not make staging directory")
				err = os.WriteFile(filepath.Join(tufDir, "staged", "targets", b, runtime.GOOS, binaryFileName), []byte("I am a test target"), 0777)
				require.NoError(t, err, "could not write test target binary to temp dir")
			}

			// Add the target
			require.NoError(t, repo.AddTarget(fmt.Sprintf("%s/%s/%s", b, runtime.GOOS, binaryFileName), nil), "could not add test target binary to tuf")

			// Commit
			require.NoError(t, repo.Snapshot(), "could not take snapshot")
			require.NoError(t, repo.Timestamp(), "could not take timestamp")
			require.NoError(t, repo.Commit(), "could not commit")

			if v != testReleaseVersion {
				continue
			}

			// If this is our release version, also create and commit a test release file
			for _, c := range []string{"stable", "beta", "nightly"} {
				require.NoError(t, os.MkdirAll(filepath.Join(tufDir, "staged", "targets", b, runtime.GOOS, c), 0777), "could not make staging directory")
				err = os.WriteFile(filepath.Join(tufDir, "staged", "targets", b, runtime.GOOS, c, "release.json"), []byte("{}"), 0777)
				require.NoError(t, err, "could not write test target release file to temp dir")
				customMetadata := fmt.Sprintf("{\"target\":\"%s/%s/%s\"}", b, runtime.GOOS, binaryFileName)
				require.NoError(t, repo.AddTarget(fmt.Sprintf("%s/%s/%s/release.json", b, runtime.GOOS, c), []byte(customMetadata)), "could not add test target release file to tuf")

				// Commit
				require.NoError(t, repo.Snapshot(), "could not take snapshot")
				require.NoError(t, repo.Timestamp(), "could not take timestamp")
				require.NoError(t, repo.Commit(), "could not commit")
			}
		}
	}

	// Quick validation that we set up the repo properly: key and metadata files should exist; targets should exist
	require.DirExists(t, filepath.Join(tufDir, "keys"))
	require.FileExists(t, filepath.Join(tufDir, "keys", "root.json"))
	require.FileExists(t, filepath.Join(tufDir, "keys", "snapshot.json"))
	require.FileExists(t, filepath.Join(tufDir, "keys", "targets.json"))
	require.FileExists(t, filepath.Join(tufDir, "keys", "timestamp.json"))
	require.DirExists(t, filepath.Join(tufDir, "repository"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "root.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "snapshot.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "timestamp.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "targets.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "targets", "launcher", runtime.GOOS, "stable", "release.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "targets", "launcher", runtime.GOOS, fmt.Sprintf("launcher-%s.tar.gz", testReleaseVersion)))
	require.FileExists(t, filepath.Join(tufDir, "repository", "targets", "osqueryd", runtime.GOOS, "stable", "release.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "targets", "osqueryd", runtime.GOOS, fmt.Sprintf("osqueryd-%s.tar.gz", testReleaseVersion)))

	// Set up a test server to serve these files
	testMetadataServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathComponents := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")

		fileToServe := tufDir

		// Allow the test server to also stand in for dl.kolide.co
		if pathComponents[0] == "kolide" {
			fileToServe = filepath.Join(fileToServe, "repository", "targets")
		} else {
			fileToServe = filepath.Join(fileToServe, pathComponents[0])
		}

		for i := 1; i < len(pathComponents); i += 1 {
			fileToServe = filepath.Join(fileToServe, pathComponents[i])
		}

		http.ServeFile(w, r, fileToServe)
	}))

	// Make sure we close the server at the end of our test
	t.Cleanup(func() {
		testMetadataServer.Close()
	})

	tufServerURL = testMetadataServer.URL

	metadata, err := repo.GetMeta()
	require.NoError(t, err, "could not get metadata from test TUF repo")
	require.Contains(t, metadata, "root.json")
	rootJson = metadata["root.json"]

	return tufServerURL, rootJson
}

func compress(t *testing.T, outFileName string, outFileDir string, targetDir string, binary string) {
	out, err := os.Create(filepath.Join(outFileDir, outFileName))
	require.NoError(t, err, "creating archive: %s in %s", outFileName, outFileDir)
	defer out.Close()

	gw := gzip.NewWriter(out)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	srcFilePath := binary
	if binary == "launcher" && runtime.GOOS == "darwin" {
		srcFilePath = filepath.Join("Kolide.app", "Contents", "MacOS", binary)

		// Create directory structure for app bundle
		for _, path := range []string{"Kolide.app", "Kolide.app/Contents", "Kolide.app/Contents/MacOS"} {
			pInfo, err := os.Stat(filepath.Join(targetDir, path))
			require.NoError(t, err, "stat for app bundle path %s", path)

			hdr, err := tar.FileInfoHeader(pInfo, path)
			require.NoError(t, err, "creating header for directory %s", path)
			hdr.Name = path

			require.NoError(t, tw.WriteHeader(hdr), "writing tar header")
		}
	} else if runtime.GOOS == "windows" {
		srcFilePath += ".exe"
	}

	srcFile, err := os.Open(filepath.Join(targetDir, srcFilePath))
	require.NoError(t, err, "opening binary")
	defer srcFile.Close()

	srcStats, err := srcFile.Stat()
	require.NoError(t, err, "getting stats to compress binary")

	hdr, err := tar.FileInfoHeader(srcStats, srcStats.Name())
	require.NoError(t, err, "creating header")
	hdr.Name = srcFilePath

	require.NoError(t, tw.WriteHeader(hdr), "writing tar header")
	_, err = io.Copy(tw, srcFile)
	require.NoError(t, err, "copying file to archive")
}

// executableLocation returns the path to the executable in `updateDirectory`.
func executableLocation(updateDirectory string, binary string) string {
	switch runtime.GOOS {
	case "darwin":
		switch binary {
		case "launcher":
			return filepath.Join(updateDirectory, "Kolide.app", "Contents", "MacOS", binary)
		case "osqueryd":
			return filepath.Join(updateDirectory, binary)
		default:
			return ""
		}
	case "windows":
		return filepath.Join(updateDirectory, fmt.Sprintf("%s.exe", binary))
	case "linux":
		return filepath.Join(updateDirectory, binary)
	default:
		return filepath.Join(updateDirectory, binary)
	}
}
