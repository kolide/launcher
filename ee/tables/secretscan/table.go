package secretscan

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fatih/semgroup"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/spf13/viper"
	"github.com/zricethezav/gitleaks/v8/config"
	"github.com/zricethezav/gitleaks/v8/detect"
	"github.com/zricethezav/gitleaks/v8/report"
	"github.com/zricethezav/gitleaks/v8/sources"
)

type contextKey string

const (
	tableName = "kolide_secret_scan"

	// directoryScanConcurrency is the number of concurrent file scans when scanning a directory
	directoryScanConcurrency = 4

	// redactPrefixLength is the number of characters to show before redacting a secret
	redactPrefixLength = 3

	// key for passing the requested salt through context
	argon2idSaltKey contextKey = "argon2idSalt"
)

func newDefaultConfig() (config.Config, error) {
	v := viper.New() // init viper here so we don't update a global var
	v.SetConfigType("toml")
	err := v.ReadConfig(strings.NewReader(config.DefaultConfig))
	if err != nil {
		return config.Config{}, err
	}
	var vc config.ViperConfig
	err = v.Unmarshal(&vc)
	if err != nil {
		return config.Config{}, err
	}
	cfg, err := vc.Translate()
	if err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

type Table struct {
	slogger       *slog.Logger
	defaultConfig *config.Config
	configOnce    sync.Once
	configErr     error
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
		table.TextColumn("redacted_secret"),
		table.TextColumn("hash_argon2id"),
		table.TextColumn("hash_argon2id_salt"),
	}

	t := &Table{
		slogger: slogger.With("table", tableName),
	}

	return tablewrapper.New(flags, slogger, tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	t.configOnce.Do(func() {
		cfg, err := newDefaultConfig()
		if err != nil {
			t.configErr = fmt.Errorf("creating default config: %w", err)
			return
		}
		t.defaultConfig = &cfg
	})
	if t.configErr != nil {
		return nil, t.configErr
	}

	var results []map[string]string

	// If we have a salt, pass it along
	ctx = context.WithValue(
		ctx,
		argon2idSaltKey,
		tablehelpers.GetConstraints(queryContext, "hash_argon2id_salt", tablehelpers.WithDefaults(""))[0],
	)

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
			pathResults, err := t.scanPath(ctx, targetPath)
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
		rawResults, err := t.scanContent(ctx, []byte(rawData))
		if err != nil {
			t.slogger.Log(ctx, slog.LevelWarn,
				"failed to scan content",
				"err", err,
			)
			continue
		}
		for i := range rawResults {
			// Return original value so SQLite WHERE clause filtering works correctly
			rawResults[i]["raw_data"] = rawData
		}
		results = append(results, rawResults...)
	}

	return results, nil
}

func (t *Table) scanPath(ctx context.Context, targetPath string) ([]map[string]string, error) {
	info, err := os.Stat(targetPath)
	if err != nil {
		return nil, fmt.Errorf("stat path: %w", err)
	}

	// Only allow regular files and directories - reject symlinks, FIFOs, sockets, devices, etc.
	if !info.IsDir() && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("unsupported file type: %s", info.Mode().Type())
	}

	// Fresh detector per scan - gitleaks accumulates findings internally
	detector := detect.NewDetector(*t.defaultConfig)

	var source sources.Source
	var file *os.File
	findingsPath := targetPath

	if info.IsDir() {
		source = &sources.Files{
			Path:           targetPath,
			Config:         &detector.Config,
			FollowSymlinks: false,
			Sema:           semgroup.NewGroup(ctx, directoryScanConcurrency),
		}
		findingsPath = "" // Directory scans use path from findings
	} else {
		file, err = os.Open(targetPath)
		if err != nil {
			return nil, fmt.Errorf("opening file: %w", err)
		}
		defer file.Close()

		source = &sources.File{
			Content: file,
			Path:    targetPath,
			Config:  &detector.Config,
		}
	}

	findings, err := detector.DetectSource(ctx, source)
	if err != nil {
		return nil, fmt.Errorf("scanning path: %w", err)
	}

	return t.findingsToRows(ctx, findings, findingsPath), nil
}

func (t *Table) scanContent(ctx context.Context, content []byte) ([]map[string]string, error) {
	// Fresh detector per scan - gitleaks accumulates findings internally
	detector := detect.NewDetector(*t.defaultConfig)

	fileSource := &sources.File{
		Content: strings.NewReader(string(content)),
		Config:  &detector.Config,
	}

	findings, err := detector.DetectSource(ctx, fileSource)
	if err != nil {
		return nil, fmt.Errorf("scanning content: %w", err)
	}

	return t.findingsToRows(ctx, findings, ""), nil
}

func (t *Table) findingsToRows(ctx context.Context, findings []report.Finding, path string) []map[string]string {
	results := make([]map[string]string, 0, len(findings))

	argon2idSalt, saltIsString := ctx.Value(argon2idSaltKey).(string)
	keepHashing := saltIsString

	for _, f := range findings {
		// Get the hash of this secret. If there's an error, log it, and allow the rest of the data to be returned.
		// But note that there's an error, since it's probably a salting issue, and we don't need to log a billion times.
		var argon2idHash string
		if keepHashing {
			var err error
			argon2idHash, err = generateArgon2idHash(f.Match, argon2idSalt)
			if err != nil {
				keepHashing = false
				t.slogger.Log(ctx, slog.LevelWarn, "error hashing", "error", err)
			}
		}

		filePath := path
		if filePath == "" {
			filePath = f.File
		}
		row := map[string]string{
			"path":               filePath,
			"raw_data":           "",
			"rule_id":            f.RuleID,
			"description":        f.Description,
			"line_number":        fmt.Sprintf("%d", f.StartLine),
			"column_start":       fmt.Sprintf("%d", f.StartColumn),
			"column_end":         fmt.Sprintf("%d", f.EndColumn),
			"entropy":            fmt.Sprintf("%.2f", f.Entropy),
			"redacted_secret":    redact(f.Match),
			"hash_argon2id":      argon2idHash,
			"hash_argon2id_salt": argon2idSalt,
		}
		results = append(results, row)
	}

	return results
}

func redact(secret string) string {
	// Only show prefix if secret is long enough that we're not revealing too much
	if len(secret) <= redactPrefixLength*2 {
		return "***"
	}
	return secret[:redactPrefixLength] + "..."
}
