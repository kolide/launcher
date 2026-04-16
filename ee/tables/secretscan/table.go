package secretscan

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/fatih/semgroup"
	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/kolide/launcher/v2/ee/tables/tablehelpers"
	"github.com/kolide/launcher/v2/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/spf13/viper"
	"github.com/zricethezav/gitleaks/v8/config"
	"github.com/zricethezav/gitleaks/v8/detect"
	"github.com/zricethezav/gitleaks/v8/report"
	"github.com/zricethezav/gitleaks/v8/sources"
)

const (
	tableName = "kolide_secret_scan"

	// directoryScanConcurrency is the number of concurrent file scans when scanning a directory
	directoryScanConcurrency = 4
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
		table.TextColumn("name"),
		table.TextColumn("path"),
		table.TextColumn("raw_data"),
		table.TextColumn("rule_id"),
		table.TextColumn("description"),
		table.IntegerColumn("line_number"),
		table.IntegerColumn("column_start"),
		table.IntegerColumn("column_end"),
		table.TextColumn("entropy"),
		table.TextColumn("hash_argon2id"),
		table.TextColumn("hash_argon2id_salt"),
	}

	t := &Table{
		slogger: slogger.With("table", tableName),
	}

	return tablewrapper.New(flags, slogger, tableName, columns, t.generate,
		tablewrapper.WithDescription("Scans files or raw content for leaked secrets using gitleaks rules. Requires a WHERE path = or raw_data = constraint. Returns rule matches with line numbers. Useful for detecting accidentally committed credentials or API keys."),
		tablewrapper.WithNote("The hash_argon2id column provides a privacy-preserving way to track whether the same secret appears across devices without exposing the secret itself. It returns only 3 bytes (6 hex characters), which is enough for rough uniqueness comparison but not enough to reverse the secret. To enable hashing, provide a WHERE hash_argon2id_salt = constraint with a 16-byte random salt, base64-encoded. The salt should be unique per organization and not predictable. If no salt is provided, the hash column will be empty."),
	)
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

	argon2idSalts := tablehelpers.GetConstraints(queryContext, "hash_argon2id_salt", tablehelpers.WithDefaults(""))
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
			pathResults, err := t.scanPath(ctx, argon2idSalts, targetPath)
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
		rawResults, err := t.scanContent(ctx, argon2idSalts, []byte(rawData))
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

func (t *Table) scanPath(ctx context.Context, argon2idSalts []string, targetPath string) ([]map[string]string, error) {
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

	return t.findingsToRows(ctx, argon2idSalts, findings, findingsPath), nil
}

func (t *Table) scanContent(ctx context.Context, argon2idSalts []string, content []byte) ([]map[string]string, error) {
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

	return t.findingsToRows(ctx, argon2idSalts, findings, ""), nil
}

func (t *Table) findingsToRows(ctx context.Context, argon2idSalts []string, findings []report.Finding, path string) []map[string]string {
	results := make([]map[string]string, 0, len(findings))

	keepHashing := true
	argon2idSalt := ""
	if len(argon2idSalts) == 1 {
		argon2idSalt = argon2idSalts[0]
	} else {
		t.slogger.Log(ctx, slog.LevelWarn,
			"got unexpected number of salts, only support 1",
			"count", len(argon2idSalts),
		)
		keepHashing = false
	}

	keyNamesInFindings := findingsToKeyNames(findings)

	// Just for logging purposes -- we're curious how frequently we detect false positives
	encryptedJwtFalsePositiveCount := 0
	emptyVariableFalsePositiveCount := 0
	for idx, f := range findings {
		// We sometimes see false positives under the "generic-api-key" rule.
		// Check for these.
		if f.RuleID == "generic-api-key" {
			if isEncryptedJWTFamilyValue(f) {
				encryptedJwtFalsePositiveCount += 1
				continue
			}
			if isEmptyVariable(f) {
				emptyVariableFalsePositiveCount += 1
				continue
			}
		}

		// Get the hash of this secret. If there's an error, log it, and allow the rest of the data to be returned.
		// But note that there's an error, since it's probably a salting issue, and we don't need to log a billion times.
		var argon2idHash string
		if keepHashing {
			var err error
			argon2idHash, err = generateArgon2idHash(f.Match, argon2idSalt)
			if err != nil {
				keepHashing = false
				t.slogger.Log(ctx, slog.LevelWarn, "error hashing", "err", err)
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
			"hash_argon2id":      argon2idHash,
			"hash_argon2id_salt": argon2idSalt,
			"name":               keyNamesInFindings[idx],
		}
		results = append(results, row)
	}

	if encryptedJwtFalsePositiveCount > 0 || emptyVariableFalsePositiveCount > 0 {
		t.slogger.Log(ctx, slog.LevelInfo,
			"detected and skipped false positive generic-api-key findings",
			"jwt_family_count", encryptedJwtFalsePositiveCount,
			"empty_variable", emptyVariableFalsePositiveCount,
		)
	}

	return results
}

type jwtFamilyHeader struct {
	Alg string `json:"alg,omitempty"`
	Enc string `json:"enc,omitempty"`
	Cty string `json:"cty,omitempty"`
}

// isEncryptedJWTFamilyValue inspects the given finding to determine if it is encrypted content
// somewhere in the JWT family (JWE or encrypted JWK). For JWEs, we only support the compact
// serialization format currently.
func isEncryptedJWTFamilyValue(finding report.Finding) bool {
	// Grab just the header to examine.
	header, _, _ := strings.Cut(finding.Secret, ".")

	// We expect these values to be b64-encoded.
	decoded, err := base64.RawURLEncoding.DecodeString(header)
	if err != nil {
		// Not base64 -- not a JWE/JWK
		return false
	}

	var h jwtFamilyHeader
	if err := json.Unmarshal(decoded, &h); err != nil {
		// Not a valid header -- not a JWE/JWK
		return false
	}

	// Check for JWEs. The presence of the Algorithm and Encryption Algorithm
	// header params suggest that this is a JWE.
	// https://datatracker.ietf.org/doc/html/rfc7516#section-4.1.1
	// https://datatracker.ietf.org/doc/html/rfc7516#section-4.1.2
	if len(h.Alg) > 0 && len(h.Enc) > 0 {
		return true
	}
	// Check for encrypted JWKs.
	// https://datatracker.ietf.org/doc/html/rfc7517#section-7
	if strings.Contains(h.Cty, "jwk+json") {
		return true
	}

	return false
}

// emptyVariableRegexp matches strings that start with a word char,
// contain only word chars and underscores or hyphens, and end with a
// singular equal sign -- for example, `MY_ENV_VAR=`.
var emptyVariableRegexp = regexp.MustCompile(`^\w[\w-]*=$`)

// isEmptyVariable inspects the given finding to determine if it is actually
// an empty variable name instead.
func isEmptyVariable(finding report.Finding) bool {
	// This type of false positive typically has an entropy score around 3,
	// so we exclude higher-entropy values right off the bat.
	if finding.Entropy >= 4 {
		return false
	}

	// Next, check for our regex match.
	if !emptyVariableRegexp.MatchString(finding.Secret) {
		return false
	}

	// We expect that this "secret" would be at the start of a line, with either nothing
	// or whitespace in front of it. However, sometimes our finding.Line will contain
	// multiple lines -- in this case, it looks like "\nMY_ENV_VAR1=\nMY_ENV_VAR2=".
	// So first we isolate the actual line we're looking at, then check to see if there's
	// anything besides whitespace in front of it.
	lines := strings.Split(strings.ReplaceAll(finding.Line, "\r\n", "\n"), "\n")
	var lineWithSecret string
	for _, line := range lines {
		if strings.Contains(line, finding.Secret) {
			lineWithSecret = line
			break
		}
	}
	if lineWithSecret == "" {
		return false
	}
	before, _, _ := strings.Cut(lineWithSecret, finding.Secret)
	beforeTrimmed := strings.TrimSpace(before)
	return beforeTrimmed == ""
}

// findingsToKeyNames attempts to extract the key names (eg: in an .env file) to help understand the context
// of the discovered secret. Because of the multitude of possible ways people can stash secrets, and the myriad of
// secret types, this is very hard to get right. So instead, we aim to solve the simple case, and ignore the rest.
func findingsToKeyNames(findings []report.Finding) []string {
	// Perticular problems...
	//
	// We can't use f.StartColumn, because in the case of CONFIG_KEY=abc123, it returns the start of CONFIG_KEY, not abc
	// So instead we remove the secret, from the match block, and then strip out any characters we don't want.
	//
	// f.Line seems appealing, but a line contains more than one secret, we cannot cleanly get the prior one.
	//
	// The behavior of f.Match is inconsistent across rules -- for the `generic-api-key` rule, it includes the KEY=
	// portion, but for an explicit rule (ie: `npm-access-token`) it does not.
	//
	// These combine to make it hard to write a generic routine

	keyNames := make([]string, len(findings))

	if len(findings) == 0 {
		return keyNames
	}

	// Because we're trying to guess the key name by looking at prior token on the line, we only want to skip lines where
	// multiple secrets are detected. (eg: `KEY1=foo KEY2=bar` or a dense json block). There are probably ways to work on
	// a multiple secret line, but let's do the "easy" thing
	secretsPerLine := make(map[int]int)
	for _, finding := range findings {
		secretsPerLine[finding.StartLine] += 1
	}

	// Now, we can finally examine the findings and try to get the key name
	for idx, finding := range findings {
		// check that we only have 1 secret on this line
		if secretsPerLine[finding.StartLine] != 1 {
			continue
		}

		// To find the token _prior_ to the secret, we split on the secret, and then take the first word
		beforeSecret, _, found := strings.Cut(finding.Line, finding.Secret)
		if !found || beforeSecret == "" {
			// this is a weird case, probably an error, but we may as well try to detect it
			continue
		}

		// Next up, replace everything we don't want with spaces
		cleanedStr := strings.Trim(nonAllowedInKeyNames.ReplaceAllString(beforeSecret, " "), " ")

		// And finally, let's find the _last_ word
		lastSpaceIndex := strings.LastIndex(cleanedStr, " ")

		if lastSpaceIndex != -1 {
			lastWord := cleanedStr[lastSpaceIndex+1:]
			keyNames[idx] = lastWord
		} else {
			// Handle the case where there are no spaces (single word or empty string)
			keyNames[idx] = cleanedStr
		}
	}

	return keyNames
}

var nonAllowedInKeyNames = regexp.MustCompile(`[^a-zA-Z0-9+./_^ -]+`)
