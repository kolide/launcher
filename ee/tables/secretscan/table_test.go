package secretscan

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/tables/ci"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSecretScan(t *testing.T) {
	t.Parallel()

	// Extract test data once for all subtests
	projectDir := extractTestData(t)

	for _, tt := range []struct {
		name string
		// Input configuration
		scanType          string                    // "path" or "raw_data"
		targetPath        string                    // relative path within test_data (for file/directory) or raw content
		setupFunc         func(t *testing.T) string // optional setup that returns absolute path to scan
		minFindingsCount  int
		expectedRuleIDs   []string // rule IDs we expect to find (at least one)
		expectedFileNames []string // file names we expect in findings (at least one)
		forbiddenFiles    []string // file names that should NOT appear in findings
	}{
		{
			name:              "scan directory finds multiple secrets",
			scanType:          "path",
			targetPath:        "", // root of test_data
			minFindingsCount:  2,
			expectedRuleIDs:   []string{"slack-bot-token", "github-pat"},
			expectedFileNames: []string{"config.yaml", "github_token.env"},
			forbiddenFiles:    []string{"clean_file.txt", "low_entropy.txt"},
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
			targetPath:        "subdir/github_token.env", // Will be converted via filepath.FromSlash
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
			name:       "scan low entropy AWS example keys finds no secrets",
			scanType:   "path",
			targetPath: "low_entropy.txt", // Contains AWS EXAMPLE keys which are low entropy and should be ignored
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
			targetPath: "nonexistent_file_that_does_not_exist.txt", // Relative path that won't exist
		},
		{
			name:     "symlink to file with secret is scanned",
			scanType: "path",
			setupFunc: func(t *testing.T) string {
				tempDir := t.TempDir()
				secretFile := filepath.Join(tempDir, "secret.txt")
				err := os.WriteFile(secretFile, []byte(`slack_token: xoxb-9876543210-9876543210-zyxwvutsrqponmlk`), 0644)
				require.NoError(t, err)
				symlinkPath := filepath.Join(tempDir, "secret_link")
				err = os.Symlink(secretFile, symlinkPath)
				require.NoError(t, err)
				return symlinkPath
			},
			minFindingsCount:  1,
			expectedRuleIDs:   []string{"slack-bot-token"},
			expectedFileNames: []string{"secret_link"},
		},
		{
			name:     "broken symlink returns empty",
			scanType: "path",
			setupFunc: func(t *testing.T) string {
				tempDir := t.TempDir()
				brokenLink := filepath.Join(tempDir, "broken_link")
				err := os.Symlink(filepath.Join(tempDir, "nonexistent"), brokenLink)
				require.NoError(t, err)
				return brokenLink
			},
			// Broken symlink causes stat error, which is logged and skipped (returns empty)
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tbl := &Table{
				slogger: multislogger.NewNopLogger(),
			}

			var queryContext map[string][]string

			switch tt.scanType {
			case "path":
				var fullPath string
				if tt.setupFunc != nil {
					// Use custom setup function to create test fixtures
					fullPath = tt.setupFunc(t)
				} else if tt.targetPath == "" {
					fullPath = projectDir
				} else if !filepath.IsAbs(tt.targetPath) {
					// Convert forward slashes to OS-specific separator
					fullPath = filepath.Join(projectDir, filepath.FromSlash(tt.targetPath))
				} else {
					fullPath = tt.targetPath
				}
				queryContext = map[string][]string{"path": {fullPath}}
			case "raw_data":
				queryContext = map[string][]string{"raw_data": {tt.targetPath}}
			default:
				t.Fatalf("unknown scanType: %s", tt.scanType)
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
				assert.Len(t, row["secret_hash"], 64, "secret_hash should be a 64-char hex SHA-256")

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

func TestHashSecret(t *testing.T) {
	t.Parallel()

	hash1 := hashSecret("mysecret", "key=mysecret")
	hash2 := hashSecret("mysecret", "key=mysecret")
	hash3 := hashSecret("different", "key=different")

	assert.Len(t, hash1, 64, "SHA-256 hex should be 64 chars")
	assert.Equal(t, hash1, hash2, "same input should produce same hash")
	assert.NotEqual(t, hash1, hash3, "different inputs should produce different hashes")
	assert.Equal(t, "652c7dc687d98c9889304ed2e408c74b611e86a40caa51c4b43f1dd5913c5cd0", hashSecret("mysecret", "ignored"))

	// Falls back to match when secret is empty
	fallback := hashSecret("", "the-match-value")
	direct := hashSecret("the-match-value", "ignored")
	assert.Equal(t, fallback, direct, "empty secret should fall back to match")
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
// Returns the path to the extracted test_data directory.
// Uses testing.TB interface to work with both *testing.T and *testing.B.
func extractTestData(tb testing.TB) string {
	tb.Helper()

	tempDir := tb.TempDir()

	// Use filepath.FromSlash for cross-platform compatibility
	zipPath := filepath.FromSlash("test_data/test_data.zip")
	zipReader, err := zip.OpenReader(zipPath)
	require.NoError(tb, err, "opening zip file")
	defer zipReader.Close()

	for _, fileInZip := range zipReader.File {
		// Zip files always use forward slashes - convert to OS separator
		// Also clean the path to remove any potential path traversal
		cleanName := filepath.FromSlash(fileInZip.Name)
		if strings.Contains(cleanName, "..") {
			continue // Skip potentially malicious paths
		}
		filePath := filepath.Join(tempDir, cleanName)

		if fileInZip.FileInfo().IsDir() {
			require.NoError(tb, os.MkdirAll(filePath, 0755), "creating dir")
			continue
		}

		require.NoError(tb, os.MkdirAll(filepath.Dir(filePath), 0755), "creating parent dir")

		fileInZipReader, err := fileInZip.Open()
		require.NoError(tb, err, "opening file in zip")

		// Use 0644 as a safe default permission that works on all platforms
		outFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		require.NoError(tb, err, "opening output file")

		_, err = io.Copy(outFile, fileInZipReader)
		fileInZipReader.Close()
		outFile.Close()
		require.NoError(tb, err, "copying from zip to temp dir")
	}

	return filepath.Join(tempDir, "test_data")
}

// Benchmarks

func BenchmarkSecretScanDirectory(b *testing.B) {
	projectDir := extractTestData(b)

	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()

	secretScanTable := TablePlugin(mockFlags, multislogger.NewNopLogger())
	require.NotNil(b, secretScanTable)

	baselineStats := ci.BaselineStats(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		response := secretScanTable.Call(b.Context(), ci.BuildRequestWithSingleEqualConstraint("path", projectDir))
		require.Equal(b, int32(0), response.Status.Code, response.Status.Message)
	}

	ci.ReportNonGolangMemoryUsage(b, baselineStats)
}

func BenchmarkSecretScanFile(b *testing.B) {
	projectDir := extractTestData(b)
	filePath := filepath.Join(projectDir, "config.yaml")

	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()

	secretScanTable := TablePlugin(mockFlags, multislogger.NewNopLogger())
	require.NotNil(b, secretScanTable)

	baselineStats := ci.BaselineStats(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		response := secretScanTable.Call(b.Context(), ci.BuildRequestWithSingleEqualConstraint("path", filePath))
		require.Equal(b, int32(0), response.Status.Code, response.Status.Message)
	}

	ci.ReportNonGolangMemoryUsage(b, baselineStats)
}

func BenchmarkSecretScanRawData(b *testing.B) {
	// Use single-line data to avoid JSON escaping issues in the request builder
	rawData := `slack_bot_token: xoxb-9876543210-9876543210-zyxwvutsrqponmlk`
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()

	secretScanTable := TablePlugin(mockFlags, multislogger.NewNopLogger())
	require.NotNil(b, secretScanTable)

	baselineStats := ci.BaselineStats(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		response := secretScanTable.Call(b.Context(), ci.BuildRequestWithSingleEqualConstraint("raw_data", rawData))
		require.Equal(b, int32(0), response.Status.Code, response.Status.Message)
	}

	ci.ReportNonGolangMemoryUsage(b, baselineStats)
}
