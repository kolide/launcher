package secretscan

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/zricethezav/gitleaks/v8/detect"
	"github.com/zricethezav/gitleaks/v8/report"
)

const tableName = "kolide_secret_scan"

// highSeverityRules contains rule IDs that are considered high severity
var highSeverityRules = map[string]bool{
	"aws-access-token":          true,
	"aws-secret-access-key":     true,
	"github-pat":                true,
	"github-fine-grained-pat":   true,
	"github-oauth":              true,
	"gitlab-pat":                true,
	"gcp-api-key":               true,
	"google-api-key":            true,
	"private-key":               true,
	"generic-api-key":           true,
	"slack-bot-token":           true,
	"slack-user-token":          true,
	"slack-webhook-url":         true,
	"stripe-access-token":       true,
	"twilio-api-key":            true,
	"sendgrid-api-token":        true,
	"mailchimp-api-key":         true,
	"npm-access-token":          true,
	"pypi-upload-token":         true,
	"azure-storage-account-key": true,
	"databricks-api-token":      true,
	"hashicorp-vault-token":     true,
	"jwt":                       true,
	"okta-access-token":         true,
	"shopify-access-token":      true,
	"telegram-bot-api-token":    true,
	"twitter-bearer-token":      true,
	"discord-webhook":           true,
	"discord-bot-token":         true,
	"doppler-api-token":         true,
	"dropbox-api-token":         true,
	"facebook-access-token":     true,
	"hubspot-api-key":           true,
	"intercom-api-key":          true,
	"linkedin-client-secret":    true,
	"mailgun-private-api-token": true,
	"planetscale-api-token":     true,
	"pulumi-api-token":          true,
	"sentry-access-token":       true,
	"snyk-api-token":            true,
	"square-access-token":       true,
	"sumologic-access-token":    true,
	"typeform-api-token":        true,
	"vault-batch-token":         true,
	"vault-service-token":       true,
	"yandex-api-key":            true,
	"zendesk-secret-key":        true,
}

// mediumSeverityRules contains rule IDs that are considered medium severity
var mediumSeverityRules = map[string]bool{
	"generic-password":   true,
	"password-in-url":    true,
	"base64-basic-auth":  true,
	"credentials-in-url": true,
}

// Table represents the secret scan table
type Table struct {
	slogger     *slog.Logger
	detector    *detect.Detector
	detectorErr error // stores initialization error to return at query time
}

// TablePlugin creates and returns the kolide_secret_scan table plugin
func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		// Input columns
		table.TextColumn("path"),
		table.TextColumn("raw_data"),
		// Output columns
		table.TextColumn("rule_id"),
		table.TextColumn("description"),
		table.TextColumn("severity"),
		table.IntegerColumn("line_number"),
		table.IntegerColumn("column_start"),
		table.IntegerColumn("column_end"),
		table.TextColumn("entropy"),
		table.TextColumn("redacted_context"),
	}

	// Initialize gitleaks detector with default config (~150 secret patterns)
	// If initialization fails, we store the error and return it at query time
	// rather than returning nil (which would cause a panic during plugin registration)
	detector, detectorErr := detect.NewDetectorDefaultConfig()
	if detectorErr != nil {
		slogger.Log(context.TODO(), slog.LevelError,
			"failed to create gitleaks detector, table will return errors when queried",
			"err", detectorErr,
		)
	}

	t := &Table{
		slogger:     slogger.With("table", tableName),
		detector:    detector,
		detectorErr: detectorErr,
	}

	return tablewrapper.New(flags, slogger, tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", tableName)
	defer span.End()

	// Return initialization error if detector failed to initialize
	if t.detectorErr != nil {
		return nil, fmt.Errorf("gitleaks detector not available: %w", t.detectorErr)
	}

	var results []map[string]string

	requestedPaths := tablehelpers.GetConstraints(queryContext, "path")
	requestedRawDatas := tablehelpers.GetConstraints(queryContext, "raw_data")

	if len(requestedPaths) == 0 && len(requestedRawDatas) == 0 {
		return results, fmt.Errorf("the %s table requires that you specify at least one of 'path' or 'raw_data'", tableName)
	}

	// Handle file paths
	for _, requestedPath := range requestedPaths {
		// Convert SQL % wildcards to glob * wildcards
		filePaths, err := filepath.Glob(strings.ReplaceAll(requestedPath, `%`, `*`))
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"bad file glob",
				"path", requestedPath,
				"err", err,
			)
			continue
		}

		for _, filePath := range filePaths {
			fileResults, err := t.scanFile(ctx, filePath)
			if err != nil {
				t.slogger.Log(ctx, slog.LevelInfo,
					"failed to scan file",
					"path", filePath,
					"err", err,
				)
				continue
			}
			results = append(results, fileResults...)
		}
	}

	// Handle raw data
	for _, rawData := range requestedRawDatas {
		rawResults := t.scanContent(ctx, []byte(rawData), "")
		// Don't echo back the raw data for security
		for i := range rawResults {
			rawResults[i]["raw_data"] = "[scanned]"
		}
		results = append(results, rawResults...)
	}

	return results, nil
}

// scanFile reads a file and scans it for secrets
func (t *Table) scanFile(ctx context.Context, filePath string) ([]map[string]string, error) {
	_, span := observability.StartSpan(ctx, "path", filePath)
	defer span.End()

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	return t.scanContent(ctx, content, filePath), nil
}

// scanContent scans byte content for secrets using gitleaks
func (t *Table) scanContent(ctx context.Context, content []byte, filePath string) []map[string]string {
	// Create a fragment for gitleaks to scan
	fragment := detect.Fragment{
		Raw:      string(content),
		FilePath: filePath,
	}

	findings := t.detector.Detect(fragment)

	return t.findingsToRows(findings, filePath)
}

// findingsToRows converts gitleaks findings to table rows
func (t *Table) findingsToRows(findings []report.Finding, path string) []map[string]string {
	results := make([]map[string]string, 0, len(findings))

	for _, f := range findings {
		row := map[string]string{
			"path":             path,
			"raw_data":         "",
			"rule_id":          f.RuleID,
			"description":      f.Description,
			"severity":         determineSeverity(f.RuleID, f.Entropy),
			"line_number":      fmt.Sprintf("%d", f.StartLine),
			"column_start":     fmt.Sprintf("%d", f.StartColumn),
			"column_end":       fmt.Sprintf("%d", f.EndColumn),
			"entropy":          fmt.Sprintf("%.2f", f.Entropy),
			"redacted_context": redact(f.Match),
		}
		results = append(results, row)
	}

	return results
}

// redact returns a redacted version of a secret for safe logging/display
// Shows first 4 characters followed by "..." to provide context without exposing the full secret
func redact(secret string) string {
	if len(secret) <= 4 {
		return "****"
	}
	return secret[:4] + "..."
}

// determineSeverity maps a rule ID and entropy to a severity level
func determineSeverity(ruleID string, entropy float32) string {
	if highSeverityRules[ruleID] {
		return "high"
	}

	if mediumSeverityRules[ruleID] {
		return "medium"
	}

	// High entropy with any rule suggests it's likely a real secret
	if entropy > 4.5 {
		return "medium"
	}

	return "low"
}
