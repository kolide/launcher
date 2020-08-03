package wix

//go:generate go-bindata -pkg testdata -o testdata/assets.go testdata/assets/

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/kolide/launcher/pkg/packagekit/wix/testdata"
	"github.com/stretchr/testify/require"
)

func TestSchemaFromHeat(t *testing.T) {
	t.Parallel()

	appFilesContent, err := testdata.Asset("testdata/assets/AppFiles.wxs")
	require.NoError(t, err)

	appFiles := &Wix{}
	require.NoError(t, xml.Unmarshal(appFilesContent, appFiles))

	// use require, not assert, so we don't traverse into a 0 length array
	require.Len(t, appFiles.Fragments, 2)
	require.Len(t, appFiles.Fragments[0].DirectoryRefs, 1)
	require.Len(t, appFiles.Fragments[0].DirectoryRefs[0].Directories, 1)
	require.Len(t, appFiles.Fragments[0].DirectoryRefs[0].Directories[0].Directories, 3)

	files := appFiles.RetFiles()
	require.Len(t, files, 6)

	foundLauncher := false
	for _, f := range files {
		if strings.HasSuffix(f.Source, "launcher.exe") {
			foundLauncher = true
			break
		}
	}

	require.True(t, foundLauncher, "Found launcher in file array")

}
