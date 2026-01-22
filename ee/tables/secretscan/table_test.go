package secretscan

import (
	"archive/zip"
	_ "embed"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zricethezav/gitleaks/v8/config"
	"github.com/zricethezav/gitleaks/v8/detect"
)

//go:embed test_data/sample_project.zip
var sampleProjectZip []byte

// Shared config to avoid concurrent viper initialization issues
var (
	sharedConfig     config.Config
	sharedConfigOnce sync.Once
	sharedConfigErr  error
)

func getSharedConfig() (config.Config, error) {
	sharedConfigOnce.Do(func() {
		detector, err := detect.NewDetectorDefaultConfig()
		if err != nil {
			sharedConfigErr = err
			return
		}
		sharedConfig = detector.Config
	})
	return sharedConfig, sharedConfigErr
}

func TestSecretScan(t *testing.T) {
	t.Parallel()

	// Extract test data once for all subtests
	tempDir := t.TempDir()
	extractTestData(t, tempDir)
	projectDir := filepath.Join(tempDir, "sample_project")

	for _, tt := range []struct {
		name string
		// Input configuration
		scanType   string // "file", "directory", or "raw_data"
		targetPath string // relative path within sample_project (for file/directory) or raw content
		// Expected outputs
		expectFindings    bool
		minFindingsCount  int
		expectedRuleIDs   []string // rule IDs we expect to find (at least one)
		expectedFileNames []string // file names we expect in findings (at least one)
		forbiddenFiles    []string // file names that should NOT appear in findings
	}{
		{
			name:              "scan directory finds multiple secrets",
			scanType:          "directory",
			targetPath:        "", // root of sample_project
			expectFindings:    true,
			minFindingsCount:  2,
			expectedRuleIDs:   []string{"slack-bot-token", "github-pat"},
			expectedFileNames: []string{"config.yaml", "github_token.env"},
			forbiddenFiles:    []string{"clean_file.txt"},
		},
		{
			name:              "scan single file with slack token",
			scanType:          "file",
			targetPath:        "config.yaml",
			expectFindings:    true,
			minFindingsCount:  1,
			expectedRuleIDs:   []string{"slack-bot-token"},
			expectedFileNames: []string{"config.yaml"},
		},
		{
			name:              "scan subdirectory file with github token",
			scanType:          "file",
			targetPath:        "subdir/github_token.env",
			expectFindings:    true,
			minFindingsCount:  1,
			expectedRuleIDs:   []string{"github-pat"},
			expectedFileNames: []string{"github_token.env"},
		},
		{
			name:             "scan clean file finds no secrets",
			scanType:         "file",
			targetPath:       "clean_file.txt",
			expectFindings:   false,
			minFindingsCount: 0,
		},
		{
			name:     "scan raw data with slack token",
			scanType: "raw_data",
			targetPath: `config:
  slack_bot_token: "xoxb-9876543210-9876543210-zyxwvutsrqponmlk"
`,
			expectFindings:   true,
			minFindingsCount: 1,
			expectedRuleIDs:  []string{"slack-bot-token"},
		},
		{
			name:             "scan raw data without secrets",
			scanType:         "raw_data",
			targetPath:       "app_name = 'my-app'\nversion = '1.0.0'\n",
			expectFindings:   false,
			minFindingsCount: 0,
		},
		{
			name:             "scan nonexistent file returns empty",
			scanType:         "file",
			targetPath:       "/nonexistent/path/to/file.txt",
			expectFindings:   false,
			minFindingsCount: 0,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tbl := createTestTable(t)

			var queryContext table.QueryContext
			var fullPath string

			switch tt.scanType {
			case "directory":
				fullPath = projectDir
				if tt.targetPath != "" {
					fullPath = filepath.Join(projectDir, tt.targetPath)
				}
				queryContext = table.QueryContext{
					Constraints: map[string]table.ConstraintList{
						"path": {
							Constraints: []table.Constraint{
								{Operator: table.OperatorEquals, Expression: fullPath},
							},
						},
					},
				}
			case "file":
				// Support absolute paths (e.g., for nonexistent file tests)
				if filepath.IsAbs(tt.targetPath) {
					fullPath = tt.targetPath
				} else {
					fullPath = filepath.Join(projectDir, tt.targetPath)
				}
				queryContext = table.QueryContext{
					Constraints: map[string]table.ConstraintList{
						"path": {
							Constraints: []table.Constraint{
								{Operator: table.OperatorEquals, Expression: fullPath},
							},
						},
					},
				}
			case "raw_data":
				queryContext = table.QueryContext{
					Constraints: map[string]table.ConstraintList{
						"raw_data": {
							Constraints: []table.Constraint{
								{Operator: table.OperatorEquals, Expression: tt.targetPath},
							},
						},
					},
				}
			}

			results, err := tbl.generate(t.Context(), queryContext)
			require.NoError(t, err)

			// Check findings count
			if tt.expectFindings {
				require.NotEmpty(t, results, "expected to find secrets")
				assert.GreaterOrEqual(t, len(results), tt.minFindingsCount,
					"expected at least %d findings, got %d", tt.minFindingsCount, len(results))
			} else {
				assert.Empty(t, results, "expected no secrets to be found")
				return
			}

			// Collect actual findings
			foundRuleIDs := make(map[string]bool)
			foundFileNames := make(map[string]bool)
			for _, row := range results {
				foundRuleIDs[row["rule_id"]] = true
				foundFileNames[filepath.Base(row["path"])] = true

				// Verify columns are properly populated
				assert.NotEmpty(t, row["rule_id"], "rule_id should be populated")
				assert.NotEmpty(t, row["description"], "description should be populated")
				assert.NotEmpty(t, row["redacted_context"], "redacted_context should be populated")
				assert.NotEqual(t, "0", row["line_number"], "line_number should be > 0")

				// For raw_data scans, verify the content is marked as scanned
				if tt.scanType == "raw_data" {
					assert.Equal(t, "[scanned]", row["raw_data"], "raw_data should be marked as scanned")
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
			expected: "AKIA...",
		},
		{
			name:     "short secret",
			input:    "abc",
			expected: "****",
		},
		{
			name:     "exactly 4 chars",
			input:    "abcd",
			expected: "****",
		},
		{
			name:     "5 chars",
			input:    "abcde",
			expected: "abcd...",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "****",
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

func extractTestData(t *testing.T, tempDir string) {
	t.Helper()

	zipFile := filepath.Join(tempDir, "sample_project.zip")
	require.NoError(t, os.WriteFile(zipFile, sampleProjectZip, 0755), "writing zip to temp dir")

	zipReader, err := zip.OpenReader(zipFile)
	require.NoError(t, err, "opening reader to zip file")
	defer zipReader.Close()

	for _, fileInZip := range zipReader.File {
		unzipFile(t, fileInZip, tempDir)
	}
}

func unzipFile(t *testing.T, fileInZip *zip.File, tempDir string) {
	t.Helper()

	fileInZipReader, err := fileInZip.Open()
	require.NoError(t, err, "opening file in zip")
	defer fileInZipReader.Close()

	filePath := filepath.Join(tempDir, fileInZip.Name)

	if fileInZip.FileInfo().IsDir() {
		require.NoError(t, os.MkdirAll(filePath, fileInZip.Mode()), "creating dir")
		return
	}

	require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0755), "creating parent dir")

	outFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileInZip.Mode())
	require.NoError(t, err, "opening output file")
	defer outFile.Close()

	_, err = io.Copy(outFile, fileInZipReader)
	require.NoError(t, err, "copying from zip to temp dir")
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
