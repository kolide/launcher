//go:build windows
// +build windows

package tuf

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	tufci "github.com/kolide/launcher/ee/tuf/ci"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
	"github.com/theupdateframework/go-tuf/data"
	"golang.org/x/sys/windows"
)

func TestAddToLibrary_WindowsACLs(t *testing.T) {
	t.Parallel()

	// Set up TUF dependencies -- we do this here to avoid re-initializing the local tuf server for each
	// binary. It's unnecessary work since the mirror serves the same data both times.
	testBaseDir := t.TempDir()
	testReleaseVersion := "1.2.4"
	tufServerUrl, rootJson := tufci.InitRemoteTufServer(t, testReleaseVersion)
	metadataClient, err := initMetadataClient(context.TODO(), testBaseDir, tufServerUrl, http.DefaultClient)
	require.NoError(t, err, "creating metadata client")
	// Re-initialize the metadata client with our test root JSON
	require.NoError(t, metadataClient.Init(rootJson), "could not initialize metadata client with test root JSON")
	_, err = metadataClient.Update()
	require.NoError(t, err, "could not update metadata client")

	// Get the target metadata
	launcherTargetMeta, err := metadataClient.Target(fmt.Sprintf("%s/%s/%s/%s-%s.tar.gz", binaryLauncher, runtime.GOOS, PlatformArch(), binaryLauncher, testReleaseVersion))
	require.NoError(t, err, "could not get test metadata for launcher target")
	osquerydTargetMeta, err := metadataClient.Target(fmt.Sprintf("%s/%s/%s/%s-%s.tar.gz", binaryOsqueryd, runtime.GOOS, PlatformArch(), binaryOsqueryd, testReleaseVersion))
	require.NoError(t, err, "could not get test metadata for launcher target")

	testCases := []struct {
		binary     autoupdatableBinary
		targetFile string
		targetMeta data.TargetFileMeta
	}{
		{
			binary:     binaryLauncher,
			targetFile: fmt.Sprintf("%s-%s.tar.gz", binaryLauncher, testReleaseVersion),
			targetMeta: launcherTargetMeta,
		},
		{
			binary:     binaryOsqueryd,
			targetFile: fmt.Sprintf("%s-%s.tar.gz", binaryOsqueryd, testReleaseVersion),
			targetMeta: osquerydTargetMeta,
		},
	}

	for _, tt := range testCases {
		tt := tt
		t.Run(string(tt.binary), func(t *testing.T) {
			t.Parallel()

			// Set up test library manager
			testLibraryManager, err := newUpdateLibraryManager(tufServerUrl, http.DefaultClient, testBaseDir, multislogger.NewNopLogger())
			require.NoError(t, err, "unexpected error creating new update library manager")

			// Request download -- make a couple concurrent requests to confirm that the lock works.
			var wg sync.WaitGroup
			for i := 0; i < 5; i += 1 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					require.NoError(t, testLibraryManager.AddToLibrary(tt.binary, "", tt.targetFile, tt.targetMeta), "expected no error adding to library")
				}()
			}

			wg.Wait()

			// Confirm the update was downloaded
			dirInfo, err := os.Stat(filepath.Join(updatesDirectory(tt.binary, testBaseDir), testReleaseVersion))
			require.NoError(t, err, "checking that update was downloaded")
			require.True(t, dirInfo.IsDir())
			executableInfo, err := os.Stat(executableLocation(filepath.Join(updatesDirectory(tt.binary, testBaseDir), testReleaseVersion), tt.binary))
			require.NoError(t, err, "checking that downloaded update includes executable")
			require.False(t, executableInfo.IsDir())

			// Confirm ACL for new directory matches parent directory
			newDirInfo, err := windows.GetNamedSecurityInfo(filepath.Join(updatesDirectory(tt.binary, testBaseDir), testReleaseVersion),
				windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
			require.NoError(t, err, "getting named security info")
			newDirDacl, _, err := newDirInfo.DACL()
			require.NoError(t, err, "getting DACL")
			require.NotNil(t, newDirDacl)

			updateDirInfo, err := windows.GetNamedSecurityInfo((updatesDirectory(tt.binary, testBaseDir)),
				windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
			require.NoError(t, err, "getting named security info")
			updateDirDacl, _, err := updateDirInfo.DACL()
			require.NoError(t, err, "getting DACL")
			require.NotNil(t, updateDirDacl)

			require.Equal(t, &newDirDacl, &updateDirDacl)
			require.Equal(t, newDirInfo.String(), updateDirInfo.String())
		})
	}
}
