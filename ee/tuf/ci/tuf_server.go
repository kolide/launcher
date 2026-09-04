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

	// Initialize TUF targets
	targets := map[string]*metadata.Metadata[metadata.TargetsType]{
		"targets": metadata.Targets(time.Now().AddDate(0, 0, 7).UTC()),
	}

	arch := runtime.GOARCH
	if runtime.GOOS == "darwin" {
		arch = "universal"
	}

	// Create test binaries and release files per binary and per release channel
	for _, b := range []string{"osqueryd", "launcher"} {
		for _, v := range []string{NonReleaseVersion, "0.12.3-deadbeef", testReleaseVersion} {
			binaryFileName := fmt.Sprintf("%s-%s.tar.gz", b, v)
			binaryDir := filepath.Join(tufDir, "repository", "targets", b, runtime.GOOS, arch)
			binaryFilepath := filepath.Join(binaryDir, binaryFileName)

			// Create a valid test binary -- an archive of an executable with the proper directory structure
			// that will actually run -- if this is the release version we care about. If this is not the
			// release version we care about, then just create a small text file since it won't be downloaded
			// and evaluated.
			if v == testReleaseVersion {
				// Create test binary and copy it to the targets directory
				require.NoError(t, os.MkdirAll(binaryDir, 0755))

				f, err := os.Create(binaryFilepath)
				require.NoError(t, err)
				_, err = f.Write(getTarballContents(t, b))
				require.NoError(t, err)
				require.NoError(t, f.Close())
			} else {
				// Create a fake test binary
				require.NoError(t, os.MkdirAll(binaryDir, 0777), "could not make staging directory")
				err := os.WriteFile(filepath.Join(tufDir, "repository", "targets", b, runtime.GOOS, arch, binaryFileName), []byte("I am a test target"), 0777)
				require.NoError(t, err, "could not write test target binary to temp dir")
			}

			// Add the target
			targetFileInfo, err := metadata.TargetFile().FromFile(binaryFilepath, "sha256")
			require.NoError(t, err)
			targetName := fmt.Sprintf("%s/%s/%s/%s", b, runtime.GOOS, arch, binaryFileName)
			targets["targets"].Signed.Targets[targetName] = targetFileInfo

			if v != testReleaseVersion {
				continue
			}

			// If this is our release version, also create and commit a test release file
			for _, c := range []string{"stable", "beta", "nightly"} {
				releaseTargetFileDir := filepath.Join(tufDir, "repository", "targets", b, runtime.GOOS, arch, c)
				releaseTargetFile := filepath.Join(releaseTargetFileDir, "release.json")
				require.NoError(t, os.MkdirAll(releaseTargetFileDir, 0777), "could not make staging directory")
				err = os.WriteFile(releaseTargetFile, []byte("{}"), 0777)
				require.NoError(t, err, "could not write test target release file to temp dir")

				customMetadata, err := json.Marshal(map[string]string{
					"target": targetName,
				})
				require.NoError(t, err)
				rawCustomMetadata := json.RawMessage(customMetadata)

				releaseFileInfo, err := metadata.TargetFile().FromFile(releaseTargetFile, "sha256")
				require.NoError(t, err)
				releaseFileInfo.Custom = &rawCustomMetadata

				targets["targets"].Signed.Targets[fmt.Sprintf("%s/%s/%s/%s/release.json", b, runtime.GOOS, arch, c)] = releaseFileInfo
			}
		}
	}

	// Initialize our other TUF roles
	snapshot := metadata.Snapshot(time.Now().AddDate(0, 0, 7).UTC())
	timestamp := metadata.Timestamp(time.Now().AddDate(0, 0, 1).UTC())
	root := metadata.Root(time.Now().AddDate(0, 0, 365).UTC())

	// Generate keys so we can sign metadata
	keys := make(map[string]ed25519.PrivateKey)
	for _, name := range []string{"targets", "snapshot", "timestamp", "root"} {
		_, private, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		keys[name] = private
		key, err := metadata.KeyFromPublicKey(private.Public())
		require.NoError(t, err)
		root.Signed.AddKey(key, name)
	}

	// Sign metadata
	for _, name := range []string{"targets", "snapshot", "timestamp", "root"} {
		key := keys[name]
		signer, err := signature.LoadSigner(key, crypto.Hash(0))
		require.NoError(t, err)
		switch name {
		case "targets":
			_, err = targets["targets"].Sign(signer)
		case "snapshot":
			_, err = snapshot.Sign(signer)
		case "timestamp":
			_, err = timestamp.Sign(signer)
		case "root":
			_, err = root.Sign(signer)
		}
		require.NoError(t, err)
	}

	// Save metadata to filesystem dir
	for _, name := range []string{"targets", "snapshot", "timestamp", "root"} {
		filename := fmt.Sprintf("%s.json", name)
		var err error
		switch name {
		case "targets":
			require.NoError(t, targets["targets"].ToFile(filepath.Join(tufDir, "repository", filename), true))
			versionedFilename := fmt.Sprintf("%d.%s.json", targets["targets"].Signed.Version, name)
			require.NoError(t, targets["targets"].ToFile(filepath.Join(tufDir, "repository", versionedFilename), true))
		case "snapshot":
			require.NoError(t, snapshot.ToFile(filepath.Join(tufDir, "repository", filename), true))
			versionedFilename := fmt.Sprintf("%d.%s.json", snapshot.Signed.Version, name)
			require.NoError(t, snapshot.ToFile(filepath.Join(tufDir, "repository", versionedFilename), true))
		case "timestamp":
			require.NoError(t, timestamp.ToFile(filepath.Join(tufDir, "repository", filename), true))
		case "root":
			require.NoError(t, root.ToFile(filepath.Join(tufDir, "repository", filename), true))
			versionedFilename := fmt.Sprintf("%d.%s.json", root.Signed.Version, name)
			require.NoError(t, root.ToFile(filepath.Join(tufDir, "repository", versionedFilename), true))
		}
		require.NoError(t, err)
	}

	// Quick validation that we set up the repo properly: metadata files should exist; targets should exist
	require.DirExists(t, filepath.Join(tufDir, "repository"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "1.root.json")) // We only performed one commit, so we can assume version 1
	require.FileExists(t, filepath.Join(tufDir, "repository", "1.snapshot.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "1.targets.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "timestamp.json")) // Timestamp file does not have versioning
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

	var err error
	rootJson, err = root.ToBytes(false)
	require.NoError(t, err, "generating root json")

	return tufServerURL, rootJson
}
