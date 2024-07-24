package indexeddb

import (
	"archive/zip"
	"context"
	_ "embed"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

//go:embed test_data/indexeddbs/file__0.indexeddb.leveldb.zip
var basicIndexeddb []byte

func TestQueryIndexeddbObjectStore(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		fileName     string
		dbName       string
		objStoreName string
		expectedRows int
		zipBytes     []byte
	}{
		{
			fileName:     "file__0.indexeddb.leveldb.zip",
			dbName:       "launchertestdb",
			objStoreName: "launchertestobjstore",
			expectedRows: 2,
			zipBytes:     basicIndexeddb,
		},
	} {
		tt := tt
		t.Run(tt.fileName, func(t *testing.T) {
			t.Parallel()

			// Write zip bytes to file
			tempDir := t.TempDir()
			zipFile := filepath.Join(tempDir, tt.fileName)
			require.NoError(t, os.WriteFile(zipFile, tt.zipBytes, 0755), "writing zip to temp dir")

			// Prepare indexeddb dir
			indexeddbDest := strings.TrimSuffix(zipFile, ".zip")
			require.NoError(t, os.MkdirAll(indexeddbDest, 0755), "creating indexeddb dir")

			// Unzip to temp dir
			zipReader, err := zip.OpenReader(zipFile)
			require.NoError(t, err, "opening reader to zip file")
			defer zipReader.Close()
			for _, fileInZip := range zipReader.File {
				fileInZipReader, err := fileInZip.Open()
				require.NoError(t, err, "opening file in zip")
				defer fileInZipReader.Close()

				idbFilePath := filepath.Join(tempDir, fileInZip.Name)

				if fileInZip.FileInfo().IsDir() {
					require.NoError(t, os.MkdirAll(idbFilePath, fileInZip.Mode()), "creating dir")
					continue
				}

				outFile, err := os.OpenFile(idbFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileInZip.Mode())
				require.NoError(t, err, "opening output file")
				defer outFile.Close()

				_, err = io.Copy(outFile, fileInZipReader)
				require.NoError(t, err, "copying from zip to temp dir")
			}

			// Perform query and check that we get the expected number of rows
			rows, err := QueryIndexeddbObjectStore(indexeddbDest, tt.dbName, tt.objStoreName)
			require.NoError(t, err, "querying indexeddb")
			require.Equal(t, tt.expectedRows, len(rows), "unexpected number of rows returned")

			// Confirm we can deserialize each row.
			slogger := multislogger.NewNopLogger()
			for _, row := range rows {
				_, err := DeserializeChrome(context.TODO(), slogger, row)
				require.NoError(t, err, "could not deserialize row")
			}
		})
	}
}
