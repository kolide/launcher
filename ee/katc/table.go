package katc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"runtime"
	"strings"

	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

const pathColumnName = "path"

// katcTable is a Kolide ATC table. It queries the source and transforms the response data
// per the configuration in its `cfg`.
type katcTable struct {
	tableName         string
	sourceType        katcSourceType
	sourcePaths       []string
	sourceQuery       string
	rowTransformSteps []rowTransformStep
	columnLookup      map[string]struct{}
	slogger           *slog.Logger
}

// newKatcTable returns a new table with the given `cfg`, as well as the osquery columns for that table.
func newKatcTable(tableName string, cfg katcTableConfig, slogger *slog.Logger) (*katcTable, []table.ColumnDefinition) {
	columns := []table.ColumnDefinition{
		{
			Name: pathColumnName,
			Type: table.ColumnTypeText,
		},
	}
	columnLookup := map[string]struct{}{
		pathColumnName: {},
	}
	for i := 0; i < len(cfg.Columns); i += 1 {
		columns = append(columns, table.ColumnDefinition{
			Name: cfg.Columns[i],
			Type: table.ColumnTypeText,
		})
		columnLookup[cfg.Columns[i]] = struct{}{}
	}

	k := katcTable{
		tableName:    tableName,
		columnLookup: columnLookup,
		slogger:      slogger,
	}

	if cfg.SourceType != nil {
		k.sourceType = *cfg.SourceType
	}
	if cfg.SourcePaths != nil {
		k.sourcePaths = *cfg.SourcePaths
	}
	if cfg.SourceQuery != nil {
		k.sourceQuery = *cfg.SourceQuery
	}
	if cfg.RowTransformSteps != nil {
		k.rowTransformSteps = *cfg.RowTransformSteps
	}

	// Check overlays to see if any of the filters apply to us;
	// use the overlay definition if so.
	for _, overlay := range cfg.Overlays {
		if !filtersMatch(overlay.Filters) {
			continue
		}

		if overlay.SourceType != nil {
			k.sourceType = *overlay.SourceType
		}
		if overlay.SourcePaths != nil {
			k.sourcePaths = *overlay.SourcePaths
		}
		if overlay.SourceQuery != nil {
			k.sourceQuery = *overlay.SourceQuery
		}
		if overlay.RowTransformSteps != nil {
			k.rowTransformSteps = *overlay.RowTransformSteps
		}

		break
	}

	// Add extra fields to slogger
	k.slogger = slogger.With(
		"table_name", tableName,
		"table_type", cfg.SourceType.String(),
	)

	return &k, columns
}

func filtersMatch(filters map[string]string) bool {
	// Currently, the only filter we expect is for OS.
	if goos, goosFound := filters["goos"]; goosFound {
		return goos == runtime.GOOS
	}
	return false
}

// generate handles queries against a KATC table.
func (k *katcTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx, "table_name", k.tableName)
	defer span.End()

	if k.sourceType.dataFunc == nil {
		return nil, errors.New("table source type not set")
	}

	// Fetch data from our table source
	dataRaw, err := k.sourceType.dataFunc(ctx, k.slogger, k.sourcePaths, k.sourceQuery, queryContext)
	if err != nil {
		k.slogger.Log(ctx, slog.LevelWarn,
			"running data func",
			"err", err,
		)
		return nil, fmt.Errorf("fetching data: %w", err)
	}

	// Process data
	transformedResults := make([]map[string]string, 0)
	for _, s := range dataRaw {
		for _, dataRawRow := range s.rows {
			// Make sure source's path is included in row data
			rowData := map[string]string{
				pathColumnName: s.path,
			}

			// Run any needed transformations on the row data
			for _, step := range k.rowTransformSteps {
				dataRawRow, err = step.transformFunc(ctx, k.slogger, dataRawRow)
				if err != nil {
					k.slogger.Log(ctx, slog.LevelWarn,
						"running transform func",
						"transform_step", step.name,
						"err", err,
					)
					return nil, fmt.Errorf("running transform func %s: %w", step.name, err)
				}
			}

			// After transformations have been applied, we can cast the data from []byte
			// to string to return to osquery.
			for key, val := range dataRawRow {
				rowData[key] = string(val)
			}
			transformedResults = append(transformedResults, rowData)
		}
	}

	// Now, filter data to ensure we only return columns in k.columnLookup
	filteredResults := make([]map[string]string, 0)
	for _, row := range transformedResults {
		filteredRow := make(map[string]string)
		for column, data := range row {
			if _, expectedColumn := k.columnLookup[column]; !expectedColumn {
				// Silently discard the column+data
				continue
			}

			filteredRow[column] = data
		}

		filteredResults = append(filteredResults, filteredRow)
	}

	return filteredResults, nil
}

// getPathConstraint retrieves any constraints against the `path` column
func getPathConstraint(queryContext table.QueryContext) *table.ConstraintList {
	if pathConstraint, pathConstraintExists := queryContext.Constraints[pathColumnName]; pathConstraintExists {
		return &pathConstraint
	}
	return nil
}

// checkPathConstraints validates whether a given `path` matches the given constraints.
func checkPathConstraints(path string, pathConstraints *table.ConstraintList) (bool, error) {
	if pathConstraints == nil {
		return true, nil
	}

	for _, pathConstraint := range pathConstraints.Constraints {
		switch pathConstraint.Operator {
		case table.OperatorEquals:
			if path != pathConstraint.Expression {
				return false, nil
			}
		case table.OperatorLike:
			// Transform the expression into a regex to test if we have a match.
			likeRegexpStr := regexp.QuoteMeta(pathConstraint.Expression)
			// % matches zero or more characters
			likeRegexpStr = strings.Replace(likeRegexpStr, "%", `.*`, -1)
			// _ matches a single character
			likeRegexpStr = strings.Replace(likeRegexpStr, "_", `.`, -1)
			// LIKE is case-insensitive
			likeRegexpStr = `(?i)` + likeRegexpStr
			r, err := regexp.Compile(likeRegexpStr)
			if err != nil {
				return false, fmt.Errorf("invalid LIKE statement: %w", err)
			}
			if !r.MatchString(path) {
				return false, nil
			}
		case table.OperatorGlob:
			// Transform the expression into a regex to test if we have a match.
			// Unlike LIKE, GLOB is case-sensitive.
			globRegexpStr := regexp.QuoteMeta(pathConstraint.Expression)
			// * matches zero or more characters
			globRegexpStr = strings.Replace(globRegexpStr, `\*`, `.*`, -1)
			// ? matches a single character
			globRegexpStr = strings.Replace(globRegexpStr, `\?`, `.`, -1)
			r, err := regexp.Compile(globRegexpStr)
			if err != nil {
				return false, fmt.Errorf("invalid GLOB statement: %w", err)
			}
			if !r.MatchString(path) {
				return false, nil
			}
		case table.OperatorRegexp:
			r, err := regexp.Compile(pathConstraint.Expression)
			if err != nil {
				return false, fmt.Errorf("invalid regex: %w", err)
			}
			if !r.MatchString(path) {
				return false, nil
			}
		default:
			return false, fmt.Errorf("operator %v not valid path constraint", pathConstraint.Operator)
		}
	}

	return true, nil
}
