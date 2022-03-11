package internal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/kit/version"
	"github.com/stretchr/testify/require"
)

func TestRecordLauncherVersion(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		additionalVerFiles int
		includeMyVersion   bool
		unrelatedFiles     int
	}{
		{additionalVerFiles: 0, includeMyVersion: false},
		{additionalVerFiles: 0, includeMyVersion: false, unrelatedFiles: 3},
		{additionalVerFiles: 1, includeMyVersion: false},
		{additionalVerFiles: 5, includeMyVersion: false},
		{additionalVerFiles: 5, includeMyVersion: false, unrelatedFiles: 3},

		{additionalVerFiles: 0, includeMyVersion: true},
		{additionalVerFiles: 1, includeMyVersion: true},
		{additionalVerFiles: 5, includeMyVersion: true},
		{additionalVerFiles: 5, includeMyVersion: true, unrelatedFiles: 3},
	}

	for _, tt := range tests {
		tt := tt
		t.Run("", func(t *testing.T) {
			t.Parallel()

			tempDir, err := os.MkdirTemp("", "record-launcher-version")
			require.NoError(t, err, "making temp dir")
			defer os.RemoveAll(tempDir)

			// Make additional files
			for i := 0; i < tt.additionalVerFiles; i++ {
				f := makeFilePath(tempDir, ulid.New())
				require.NoError(t, touchFile(f), "setting up additional file")
			}

			// unrelated files do not conform to the
			// naming scheme, so should not be deleted.
			for i := 0; i < tt.unrelatedFiles; i++ {
				f := filepath.Join(tempDir, ulid.New())
				require.NoError(t, os.WriteFile(f, nil, 0644), "setting up unrelated file")
			}

			if tt.includeMyVersion {
				f := makeFilePath(tempDir, version.Version().Version)
				require.NoError(t, touchFile(f), "setting up my version file")
			}

			// Now, test!
			require.NoError(t, RecordLauncherVersion(tempDir), "recording launcher version")
			files, err := filepath.Glob(filepath.Join(tempDir, "*"))
			require.NoError(t, err, "calling RecordLauncherVersion")
			require.Equal(t, tt.unrelatedFiles+1, len(files), "expected number of files")
		})
	}
}
