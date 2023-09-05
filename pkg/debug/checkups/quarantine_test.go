package checkups

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/kolide/kit/ulid"
	"github.com/stretchr/testify/require"
)

func Test_quarantine_checkDirForQuarantinedFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		shouldPass bool
		pathsFunc  func(t *testing.T) string
	}{
		{
			name: "empty dir",
			pathsFunc: func(t *testing.T) string {
				return t.TempDir()
			},
			shouldPass: true,
		},
		{
			name: "dir does not exist",
			pathsFunc: func(t *testing.T) string {
				return ulid.New()
			},
			shouldPass: false,
		},
		{
			name: "osquery found",
			pathsFunc: func(t *testing.T) string {
				dir := t.TempDir()
				// add some we don't care about
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "a", "b", "c"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a", "b", "dont care"), nil, 0755))

				// add some we do care about
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "1", "2", "3"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "1", "2", "osquery"), nil, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "1", "2", "3", "osquery"), nil, 0755))
				return dir
			},
			shouldPass: false,
		},
		{
			name: "no notable files",
			pathsFunc: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "a", "b", "c"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a", "b", "dont care"), nil, 0755))

				require.NoError(t, os.MkdirAll(filepath.Join(dir, "1", "2", "3"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "1", "2", "dont care"), nil, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "1", "2", "3", "dont care at all"), nil, 0755))
				return dir
			},
			shouldPass: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			q := quarantine{}
			pass, _ := q.checkDirForQuarantinedFiles(io.Discard, tt.pathsFunc(t), []string{"osquery"})
			require.Equal(t, tt.shouldPass, pass)
		})
	}
}
