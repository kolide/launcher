package secretscan

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/semgroup"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/zricethezav/gitleaks/v8/config"
	"github.com/zricethezav/gitleaks/v8/detect"
	"github.com/zricethezav/gitleaks/v8/report"
	"github.com/zricethezav/gitleaks/v8/sources"
)

const (
	tableName = "kolide_secret_scan"

	// directoryScanConcurrency is the number of concurrent file scans when scanning a directory
	directoryScanConcurrency = 4

	// redactPrefixLength is the number of characters to show before redacting a secret
	redactPrefixLength = 3
)

type Table struct {
	slogger   *slog.Logger
	config    config.Config
	configErr error
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("path"),
		table.TextColumn("raw_data"),
		table.TextColumn("rule_id"),
		table.TextColumn("description"),
		table.IntegerColumn("line_number"),
		table.IntegerColumn("column_start"),
		table.IntegerColumn("column_end"),
		table.TextColumn("entropy"),
		table.TextColumn("redacted_context"),
	}

	var cfg config.Config
	var configErr error

	tempDetector, err := detect.NewDetectorDefaultConfig()
	if err != nil {
		configErr = err
		slogger.Log(context.TODO(), slog.LevelError,
			"failed to create gitleaks config, table will return errors when queried",
			"err", err,
		)
	} else {
		cfg = tempDetector.Config
	}

	t := &Table{
		slogger:   slogger.With("table", tableName),
		config:    cfg,
		configErr: configErr,
	}

	return tablewrapper.New(flags, slogger, tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {

	if t.configErr != nil {
		return nil, fmt.Errorf("gitleaks config not available: %w", t.configErr)
	}

	// Fresh detector per query - gitleaks accumulates findings internally
	detector := detect.NewDetector(t.config)

	var results []map[string]string

	requestedPaths := tablehelpers.GetConstraints(queryContext, "path")
	requestedRawDatas := tablehelpers.GetConstraints(queryContext, "raw_data")

	if len(requestedPaths) == 0 && len(requestedRawDatas) == 0 {
		return results, fmt.Errorf("the %s table requires that you specify at least one of 'path' or 'raw_data'", tableName)
	}

	for _, requestedPath := range requestedPaths {
		expandedPaths, err := filepath.Glob(strings.ReplaceAll(requestedPath, `%`, `*`))
		if err != nil {
			t.slogger.Log(ctx, slog.LevelWarn,
				"bad file glob",
				"path", requestedPath,
				"err", err,
			)
			continue
		}

		for _, targetPath := range expandedPaths {
			pathResults, err := t.scanPath(ctx, detector, targetPath)
			if err != nil {
				t.slogger.Log(ctx, slog.LevelWarn,
					"failed to scan path",
					"path", targetPath,
					"err", err,
				)
				continue
			}
			results = append(results, pathResults...)
		}
	}

	for _, rawData := range requestedRawDatas {
		rawResults := t.scanContent(ctx, detector, []byte(rawData))
		for i := range rawResults {
			rawResults[i]["raw_data"] = "[scanned]"
		}
		results = append(results, rawResults...)
	}

	return results, nil
}

func (t *Table) scanPath(ctx context.Context, detector *detect.Detector, targetPath string) ([]map[string]string, error) {
	info, err := os.Stat(targetPath)
	if err != nil {
		return nil, fmt.Errorf("stat path: %w", err)
	}

	var findings []report.Finding

	if info.IsDir() {
		dirSource := &sources.Files{
			Path:           targetPath,
			Config:         &detector.Config,
			FollowSymlinks: false,
			Sema:           semgroup.NewGroup(ctx, directoryScanConcurrency),
		}

		findings, err = detector.DetectSource(ctx, dirSource)
		if err != nil {
			return nil, fmt.Errorf("scanning directory: %w", err)
		}

		return t.findingsToRows(findings), nil
	}

	file, err := os.Open(targetPath)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	fileSource := &sources.File{
		Content: file,
		Path:    targetPath,
		Config:  &detector.Config,
	}

	findings, err = detector.DetectSource(ctx, fileSource)
	if err != nil {
		return nil, fmt.Errorf("scanning file: %w", err)
	}

	return t.findingsToRowsWithPath(findings, targetPath), nil
}

func (t *Table) scanContent(ctx context.Context, detector *detect.Detector, content []byte) []map[string]string {
	fileSource := &sources.File{
		Content: strings.NewReader(string(content)),
		Config:  &detector.Config,
	}

	findings, err := detector.DetectSource(ctx, fileSource)
	if err != nil {
		t.slogger.Log(ctx, slog.LevelWarn,
			"failed to scan content",
			"err", err,
		)
		return nil
	}

	return t.findingsToRows(findings)
}

func (t *Table) findingsToRows(findings []report.Finding) []map[string]string {
	results := make([]map[string]string, 0, len(findings))

	for _, f := range findings {
		row := map[string]string{
			"path":             f.File,
			"raw_data":         "",
			"rule_id":          f.RuleID,
			"description":      f.Description,
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

func (t *Table) findingsToRowsWithPath(findings []report.Finding, path string) []map[string]string {
	results := make([]map[string]string, 0, len(findings))

	for _, f := range findings {
		row := map[string]string{
			"path":             path,
			"raw_data":         "",
			"rule_id":          f.RuleID,
			"description":      f.Description,
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

func redact(secret string) string {
	if len(secret) <= redactPrefixLength {
		return "***"
	}
	return secret[:redactPrefixLength] + "..."
}
