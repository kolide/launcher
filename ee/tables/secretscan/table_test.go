package secretscan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/zricethezav/gitleaks/v8/detect"
)

// Shared detector to avoid concurrent viper initialization issues
var (
	sharedDetector     *detect.Detector
	sharedDetectorOnce sync.Once
	sharedDetectorErr  error
)

func getSharedDetector() (*detect.Detector, error) {
	sharedDetectorOnce.Do(func() {
		sharedDetector, sharedDetectorErr = detect.NewDetectorDefaultConfig()
	})
	return sharedDetector, sharedDetectorErr
}

func TestTablePlugin(t *testing.T) {
	t.Parallel()

	mockFlags := mocks.NewFlags(t)
	mockFlags.On("TableGenerateTimeout").Return(4 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()

	slogger := multislogger.NewNopLogger()

	plugin := TablePlugin(mockFlags, slogger)
	require.NotNil(t, plugin)
	assert.Equal(t, tableName, plugin.Name())
}

func TestGenerate_RequiresConstraint(t *testing.T) {
	t.Parallel()

	tbl := createTestTable(t)

	// Empty query context should return error
	queryContext := table.QueryContext{
		Constraints: map[string]table.ConstraintList{},
	}

	results, err := tbl.generate(context.Background(), queryContext)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires that you specify")
	assert.Empty(t, results)
}

func TestGenerate_ScanFileWithSecret(t *testing.T) {
	t.Parallel()

	tbl := createTestTable(t)

	// Create a temp file with a Slack bot token - this pattern is reliably detected
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_secrets.txt")
	secretContent := `# Config file
slack_token = "xoxb-1234567890-1234567890-abcdefghijklmnop"
`
	err := os.WriteFile(testFile, []byte(secretContent), 0600)
	require.NoError(t, err)

	queryContext := table.QueryContext{
		Constraints: map[string]table.ConstraintList{
			"path": {
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorEquals,
						Expression: testFile,
					},
				},
			},
		},
	}

	results, err := tbl.generate(context.Background(), queryContext)
	require.NoError(t, err)

	// Should find at least one secret (the AWS key patterns)
	assert.NotEmpty(t, results, "expected to find secrets in test file")

	if len(results) > 0 {
		// Check that the result has expected columns
		firstResult := results[0]
		assert.Equal(t, testFile, firstResult["path"])
		assert.NotEmpty(t, firstResult["rule_id"])
		assert.NotEmpty(t, firstResult["description"])
		assert.NotEmpty(t, firstResult["severity"])
		assert.NotEmpty(t, firstResult["redacted_context"])

		// Verify the secret is redacted (should be 4 chars + "...")
		redacted := firstResult["redacted_context"]
		assert.True(t, len(redacted) <= 8, "redacted context should be short")
	}
}

func TestGenerate_ScanFileNoSecrets(t *testing.T) {
	t.Parallel()

	tbl := createTestTable(t)

	// Create a temp file with no secrets
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "no_secrets.txt")
	cleanContent := `# This is a config file
name = "my-app"
version = "1.0.0"
debug = true
`
	err := os.WriteFile(testFile, []byte(cleanContent), 0600)
	require.NoError(t, err)

	queryContext := table.QueryContext{
		Constraints: map[string]table.ConstraintList{
			"path": {
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorEquals,
						Expression: testFile,
					},
				},
			},
		},
	}

	results, err := tbl.generate(context.Background(), queryContext)
	require.NoError(t, err)
	assert.Empty(t, results, "should not find any secrets in clean file")
}

func TestGenerate_ScanRawData(t *testing.T) {
	t.Parallel()

	tbl := createTestTable(t)

	// Raw data with a Slack bot token - reliably detected by gitleaks
	rawData := `config:
  slack_bot_token: "xoxb-9876543210-9876543210-zyxwvutsrqponmlk"
`

	queryContext := table.QueryContext{
		Constraints: map[string]table.ConstraintList{
			"raw_data": {
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorEquals,
						Expression: rawData,
					},
				},
			},
		},
	}

	results, err := tbl.generate(context.Background(), queryContext)
	require.NoError(t, err)

	// Should find the Slack token
	assert.NotEmpty(t, results, "expected to find Slack token in raw data")

	if len(results) > 0 {
		// Verify raw_data is marked as scanned, not echoed back
		assert.Equal(t, "[scanned]", results[0]["raw_data"])
	}
}

func TestGenerate_NonexistentFile(t *testing.T) {
	t.Parallel()

	tbl := createTestTable(t)

	queryContext := table.QueryContext{
		Constraints: map[string]table.ConstraintList{
			"path": {
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorEquals,
						Expression: "/nonexistent/path/to/file.txt",
					},
				},
			},
		},
	}

	// Should not error, just return empty results (file errors are logged and skipped)
	results, err := tbl.generate(context.Background(), queryContext)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestRedact(t *testing.T) {
	t.Parallel()

	tests := []struct {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := redact(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetermineSeverity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ruleID   string
		entropy  float32
		expected string
	}{
		{
			name:     "high severity - AWS key",
			ruleID:   "aws-access-token",
			entropy:  3.0,
			expected: "high",
		},
		{
			name:     "high severity - GitHub PAT",
			ruleID:   "github-pat",
			entropy:  4.0,
			expected: "high",
		},
		{
			name:     "medium severity - generic password",
			ruleID:   "generic-password",
			entropy:  3.0,
			expected: "medium",
		},
		{
			name:     "medium severity - high entropy unknown rule",
			ruleID:   "unknown-rule",
			entropy:  5.0,
			expected: "medium",
		},
		{
			name:     "low severity - unknown rule low entropy",
			ruleID:   "unknown-rule",
			entropy:  2.0,
			expected: "low",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineSeverity(tt.ruleID, tt.entropy)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerate_DetectorError(t *testing.T) {
	t.Parallel()

	// Create a table with a simulated detector initialization error
	tbl := &Table{
		slogger:     multislogger.NewNopLogger(),
		detector:    nil,
		detectorErr: errors.New("simulated detector initialization failure"),
	}

	queryContext := table.QueryContext{
		Constraints: map[string]table.ConstraintList{
			"path": {
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorEquals,
						Expression: "/some/file.txt",
					},
				},
			},
		},
	}

	// Should return the initialization error, not panic
	results, err := tbl.generate(context.Background(), queryContext)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gitleaks detector not available")
	assert.Contains(t, err.Error(), "simulated detector initialization failure")
	assert.Nil(t, results)
}

// Helper functions

func createTestTable(t *testing.T) *Table {
	t.Helper()

	detector, err := getSharedDetector()
	require.NoError(t, err)

	return &Table{
		slogger:     multislogger.NewNopLogger(),
		detector:    detector,
		detectorErr: nil,
	}
}
