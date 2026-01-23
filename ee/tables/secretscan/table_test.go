package secretscan

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretScan(t *testing.T) {
	t.Parallel()

	// Extract test data once for all subtests
	projectDir := extractTestData(t)

	for _, tt := range []struct {
		name string
		// Input configuration
		scanType          string // "path" or "raw_data"
		targetPath        string // relative path within sample_project (for file/directory) or raw content
		minFindingsCount  int
		expectedRuleIDs   []string // rule IDs we expect to find (at least one)
		expectedFileNames []string // file names we expect in findings (at least one)
		forbiddenFiles    []string // file names that should NOT appear in findings
	}{
		{
			name:              "scan directory finds multiple secrets",
			scanType:          "path",
			targetPath:        "", // root of sample_project
			minFindingsCount:  2,
			expectedRuleIDs:   []string{"slack-bot-token", "github-pat"},
			expectedFileNames: []string{"config.yaml", "github_token.env"},
			forbiddenFiles:    []string{"clean_file.txt"},
		},
		{
			name:              "scan single file with slack token",
			scanType:          "path",
			targetPath:        "config.yaml",
			minFindingsCount:  1,
			expectedRuleIDs:   []string{"slack-bot-token"},
			expectedFileNames: []string{"config.yaml"},
		},
		{
			name:              "scan subdirectory file with github token",
			scanType:          "path",
			targetPath:        "subdir/github_token.env",
			minFindingsCount:  1,
			expectedRuleIDs:   []string{"github-pat"},
			expectedFileNames: []string{"github_token.env"},
		},
		{
			name:       "scan clean file finds no secrets",
			scanType:   "path",
			targetPath: "clean_file.txt",
		},
		{
			name:     "scan raw data with slack token",
			scanType: "raw_data",
			targetPath: `config:
  slack_bot_token: "xoxb-9876543210-9876543210-zyxwvutsrqponmlk"
`,
			minFindingsCount: 1,
			expectedRuleIDs:  []string{"slack-bot-token"},
		},
		{
			name:       "scan raw data without secrets",
			scanType:   "raw_data",
			targetPath: "app_name = 'my-app'\nversion = '1.0.0'\n",
		},
		{
			name:       "scan nonexistent file returns empty",
			scanType:   "path",
			targetPath: "/nonexistent/path/to/file.txt",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tbl := createTestTable(t)

			var queryContext map[string][]string

			switch tt.scanType {
			case "path":
				fullPath := tt.targetPath
				if tt.targetPath == "" {
					fullPath = projectDir
				} else if !filepath.IsAbs(tt.targetPath) {
					fullPath = filepath.Join(projectDir, tt.targetPath)
				}
				queryContext = map[string][]string{"path": {fullPath}}
			case "raw_data":
				queryContext = map[string][]string{"raw_data": {tt.targetPath}}
			}

			results, err := tbl.generate(t.Context(), tablehelpers.MockQueryContext(queryContext))
			require.NoError(t, err)

			// Check findings count
			if tt.minFindingsCount == 0 {
				assert.Empty(t, results, "expected no secrets to be found")
				return
			}

			require.GreaterOrEqual(t, len(results), tt.minFindingsCount,
				"expected at least %d findings, got %d", tt.minFindingsCount, len(results))

			// Collect actual findings
			foundRuleIDs := make(map[string]bool)
			foundFileNames := make(map[string]bool)
			for _, row := range results {
				foundRuleIDs[row["rule_id"]] = true
				foundFileNames[filepath.Base(row["path"])] = true

				// Verify columns are properly populated
				assert.NotEmpty(t, row["rule_id"], "rule_id should be populated")
				assert.NotEmpty(t, row["description"], "description should be populated")
				assert.NotEmpty(t, row["redacted_secret"], "redacted_secret should be populated")
				assert.NotEqual(t, "0", row["line_number"], "line_number should be > 0")

				// For raw_data scans, verify the original input is returned (for SQLite filtering to work)
				if tt.scanType == "raw_data" {
					assert.Equal(t, tt.targetPath, row["raw_data"], "raw_data should contain the original input")
				}
			}

			// Verify expected rule IDs were found
			for _, expectedRule := range tt.expectedRuleIDs {
				assert.True(t, foundRuleIDs[expectedRule],
					"expected to find rule %q, found: %v", expectedRule, foundRuleIDs)
			}

			// Verify expected file names were found
			for _, expectedFile := range tt.expectedFileNames {
				assert.True(t, foundFileNames[expectedFile],
					"expected findings in file %q, found: %v", expectedFile, foundFileNames)
			}

			// Verify forbidden files were not flagged
			for _, forbiddenFile := range tt.forbiddenFiles {
				assert.False(t, foundFileNames[forbiddenFile],
					"file %q should not have any findings", forbiddenFile)
			}
		})
	}
}

func TestRedact(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "long secret",
			input:    "AKIAIOSFODNN7EXAMPLE",
			expected: "AKI...",
		},
		{
			name:     "short secret",
			input:    "abc",
			expected: "***",
		},
		{
			name:     "exactly 6 chars redacts fully",
			input:    "abcdef",
			expected: "***",
		},
		{
			name:     "7 chars shows prefix",
			input:    "abcdefg",
			expected: "abc...",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "***",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := redact(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper functions

// extractTestData reads the test zip file from disk and extracts it to a temp directory.
// Returns the path to the extracted sample_project directory.
func extractTestData(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()

	zipReader, err := zip.OpenReader("test_data/sample_project.zip")
	require.NoError(t, err, "opening zip file")
	defer zipReader.Close()

	for _, fileInZip := range zipReader.File {
		filePath := filepath.Join(tempDir, fileInZip.Name)

		if fileInZip.FileInfo().IsDir() {
			require.NoError(t, os.MkdirAll(filePath, fileInZip.Mode()), "creating dir")
			continue
		}

		require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0755), "creating parent dir")

		fileInZipReader, err := fileInZip.Open()
		require.NoError(t, err, "opening file in zip")

		outFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileInZip.Mode())
		require.NoError(t, err, "opening output file")

		_, err = io.Copy(outFile, fileInZipReader)
		fileInZipReader.Close()
		outFile.Close()
		require.NoError(t, err, "copying from zip to temp dir")
	}

	return filepath.Join(tempDir, "sample_project")
}

func createTestTable(t *testing.T) *Table {
	t.Helper()

	cfg, err := getSharedConfig()
	require.NoError(t, err)

	return &Table{
		slogger: multislogger.NewNopLogger(),
		config:  cfg,
	}
}
