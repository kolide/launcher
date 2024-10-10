package tufci

import (
	"crypto"
	"crypto/ed25519"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/stretchr/testify/require"
	"github.com/theupdateframework/go-tuf/v2/metadata"
	"github.com/theupdateframework/go-tuf/v2/metadata/repository"
)

//go:embed testdata/*.tar.gz
var testTarballs embed.FS

const NonReleaseVersion = "0.1.1"

func getTarballContents(t *testing.T, binary string) []byte {
	tarballName := fmt.Sprintf("testdata/%s_%s.tar.gz", runtime.GOOS, binary)

	contents, err := testTarballs.ReadFile(tarballName)
	require.NoError(t, err)

	return contents
}

// InitRemoteTufServer sets up a local TUF repo with some targets to serve metadata about; returns the URL
// of a test HTTP server to serve that metadata and the root JSON needed to initialize a client.
func InitRemoteTufServer(t *testing.T, testReleaseVersion string) (tufServerURL string, rootJson []byte) {
	tufDir := t.TempDir()
	repoDir := filepath.Join(tufDir, "repository")
	require.NoError(t, os.MkdirAll(repoDir, 0777))

	// Initialize repo with store
	roles := repository.New()
	keys := map[string]ed25519.PrivateKey{}

	// Create targets
	targets := metadata.Targets(time.Now().AddDate(0, 0, 1).UTC())
	roles.SetTargets("targets", targets)

	// Add all targets
	arch := runtime.GOARCH
	if runtime.GOOS == "darwin" {
		arch = "universal"
	}

	// Create test binaries and release files per binary and per release channel
	for _, b := range []string{"osqueryd", "launcher"} {
		for _, v := range []string{NonReleaseVersion, "0.12.3-deadbeef", testReleaseVersion} {
			binaryFileName := fmt.Sprintf("%s-%s.tar.gz", b, v)
			localPath := filepath.Join(repoDir, "targets", b, runtime.GOOS, arch, binaryFileName)
			require.NoError(t, os.MkdirAll(filepath.Dir(localPath), 0755))

			// Create a valid test binary -- an archive of an executable with the proper directory structure
			// that will actually run -- if this is the release version we care about. If this is not the
			// release version we care about, then just create a small text file since it won't be downloaded
			// and evaluated.
			if v == testReleaseVersion {
				// Create test binary and copy it to the targets directory
				f, err := os.Create(localPath)
				require.NoError(t, err)
				_, err = f.Write(getTarballContents(t, b))
				require.NoError(t, err)
				require.NoError(t, f.Close())
			} else {
				// Create a fake test binary
				err := os.WriteFile(localPath, []byte("I am a test target"), 0777)
				require.NoError(t, err, "could not write test target binary to temp dir")
			}

			targetName := fmt.Sprintf("%s/%s/%s/%s", b, runtime.GOOS, arch, binaryFileName)
			targetFileInfo, err := metadata.TargetFile().FromFile(localPath, "sha256")
			require.NoError(t, err)
			roles.Targets("targets").Signed.Targets[targetName] = targetFileInfo

			if v != testReleaseVersion {
				continue
			}

			// If this is our release version, also create and commit a test release file
			for _, c := range []string{"stable", "beta", "nightly"} {
				releaseFilePath := filepath.Join(repoDir, "targets", b, runtime.GOOS, arch, c, "release.json")
				require.NoError(t, os.MkdirAll(filepath.Dir(releaseFilePath), 0777), "could not make staging directory")
				require.NoError(t, os.WriteFile(releaseFilePath, []byte("{}"), 0777), "could not write test target release file to temp dir")

				targetFileInfo, err := metadata.TargetFile().FromFile(releaseFilePath, "sha256")
				require.NoError(t, err)
				customMetadata, err := json.Marshal(map[string]string{
					"target": targetName,
				})
				require.NoError(t, err)
				targetFileInfo.Custom = (*json.RawMessage)(&customMetadata)

				roles.Targets("targets").Signed.Targets[fmt.Sprintf("%s/%s/%s/%s/release.json", b, runtime.GOOS, arch, c)] = targetFileInfo
			}
		}
	}

	snapshot := metadata.Snapshot(time.Now().AddDate(0, 0, 1).UTC())
	roles.SetSnapshot(snapshot)

	timestamp := metadata.Timestamp(time.Now().AddDate(0, 0, 1).UTC())
	roles.SetTimestamp(timestamp)

	root := metadata.Root(time.Now().AddDate(0, 0, 1).UTC())
	roles.SetRoot(root)

	// Gen keys
	for _, name := range []string{"targets", "snapshot", "timestamp", "root"} {
		_, private, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		keys[name] = private
		key, err := metadata.KeyFromPublicKey(private.Public())
		require.NoError(t, err)
		require.NoError(t, roles.Root().Signed.AddKey(key, name))
	}

	// Sign
	for _, name := range []string{"targets", "snapshot", "timestamp", "root"} {
		key := keys[name]
		signer, err := signature.LoadSigner(key, crypto.Hash(0))
		require.NoError(t, err)
		switch name {
		case "targets":
			_, err = roles.Targets("targets").Sign(signer)
		case "snapshot":
			_, err = roles.Snapshot().Sign(signer)
		case "timestamp":
			_, err = roles.Timestamp().Sign(signer)
		case "root":
			_, err = roles.Root().Sign(signer)
		}
		require.NoError(t, err)
	}

	// Save to repository
	for _, name := range []string{"targets", "snapshot", "timestamp", "root"} {
		switch name {
		case "targets":
			filename := fmt.Sprintf("%d.%s.json", roles.Targets("targets").Signed.Version, name)
			require.NoError(t, roles.Targets("targets").ToFile(filepath.Join(repoDir, filename), false))
		case "snapshot":
			filename := fmt.Sprintf("%d.%s.json", roles.Snapshot().Signed.Version, name)
			require.NoError(t, roles.Snapshot().ToFile(filepath.Join(repoDir, filename), false))
		case "timestamp":
			filename := fmt.Sprintf("%s.json", name)
			require.NoError(t, roles.Timestamp().ToFile(filepath.Join(repoDir, filename), false))
		case "root":
			versionedFilename := fmt.Sprintf("%d.%s.json", roles.Root().Signed.Version, name)
			require.NoError(t, roles.Root().ToFile(filepath.Join(repoDir, versionedFilename), false))
		}
	}

	// Quick validation that we set up the repo properly: metadata files should exist; targets should exist
	require.DirExists(t, filepath.Join(tufDir, "repository"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "1.root.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "1.snapshot.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "timestamp.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "1.targets.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "targets", "launcher", runtime.GOOS, arch, "stable", "release.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "targets", "launcher", runtime.GOOS, arch, fmt.Sprintf("launcher-%s.tar.gz", testReleaseVersion)))
	require.FileExists(t, filepath.Join(tufDir, "repository", "targets", "osqueryd", runtime.GOOS, arch, "stable", "release.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "targets", "osqueryd", runtime.GOOS, arch, fmt.Sprintf("osqueryd-%s.tar.gz", testReleaseVersion)))

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

	rootJson, err := roles.Root().ToBytes(false)
	require.NoError(t, err)

	return tufServerURL, rootJson
}
