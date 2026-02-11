package filewalker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalJSON(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName         string
		fileTypeRaw          []byte
		expectedFileTypeName string
		expectedError        bool
	}{
		{
			testCaseName:         fileTypeFile,
			fileTypeRaw:          []byte(`"file"`),
			expectedFileTypeName: fileTypeFile,
			expectedError:        false,
		},
		{
			testCaseName:         fileTypeDir,
			fileTypeRaw:          []byte(`"dir"`),
			expectedFileTypeName: fileTypeDir,
			expectedError:        false,
		},
		{
			testCaseName:         "invalid",
			fileTypeRaw:          []byte(`"notsupported"`),
			expectedFileTypeName: "",
			expectedError:        true,
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			var ft fileTypeFilter
			err := json.Unmarshal(tt.fileTypeRaw, &ft)
			if tt.expectedError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectedFileTypeName, ft.name)
		})
	}
}

func Test_fileTypeFilter_matches(t *testing.T) {
	t.Parallel()

	// Set up directory to test against
	tempDir := t.TempDir()
	dirInfo, err := os.Stat(tempDir)
	require.NoError(t, err)

	// Set up file to test against
	tempFile := filepath.Join(tempDir, uuid.NewString())
	require.NoError(t, os.WriteFile(tempFile, []byte("test"), 0755))
	fileInfo, err := os.Stat(tempFile)
	require.NoError(t, err)

	// Test fileTypeFile first
	var ftFile fileTypeFilter
	require.NoError(t, json.Unmarshal([]byte(`"file"`), &ftFile))

	require.False(t, ftFile.matches(dirInfo.Mode()))
	require.True(t, ftFile.matches(fileInfo.Mode()))

	// Test fileTypeDir next
	var ftDir fileTypeFilter
	require.NoError(t, json.Unmarshal([]byte(`"dir"`), &ftDir))

	require.True(t, ftDir.matches(dirInfo.Mode()))
	require.False(t, ftDir.matches(fileInfo.Mode()))
}
