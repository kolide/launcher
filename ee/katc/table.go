package katc

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/osquery/osquery-go/plugin/table"
)

const sourceColumnName = "source"

// katcTable is a Kolide ATC table. It queries the source and transforms the response data
// per the configuration in its `cfg`.
type katcTable struct {
	cfg          katcTableConfig
	columnLookup map[string]struct{}
	slogger      *slog.Logger
}

// newKatcTable returns a new table with the given `cfg`, as well as the osquery columns for that table.
func newKatcTable(tableName string, cfg katcTableConfig, slogger *slog.Logger) (*katcTable, []table.ColumnDefinition) {
	columns := []table.ColumnDefinition{
		{
			Name: sourceColumnName,
			Type: table.ColumnTypeText,
		},
	}
	columnLookup := map[string]struct{}{
		sourceColumnName: {},
	}
	for i := 0; i < len(cfg.Columns); i += 1 {
		columns = append(columns, table.ColumnDefinition{
			Name: cfg.Columns[i],
			Type: table.ColumnTypeText,
		})
		columnLookup[cfg.Columns[i]] = struct{}{}
	}

	return &katcTable{
		cfg:          cfg,
		columnLookup: columnLookup,
		slogger: slogger.With(
			"table_name", tableName,
			"table_type", cfg.SourceType,
			"table_source", cfg.Source,
		),
	}, columns
}

// generate handles queries against a KATC table.
func (k *katcTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	// Fetch data from our table source
	dataRaw, err := k.cfg.SourceType.dataFunc(ctx, k.slogger, k.cfg.Source, k.cfg.Query, getSourceConstraint(queryContext))
	if err != nil {
		return nil, fmt.Errorf("fetching data: %w", err)
	}

	// Process data
	transformedResults := make([]map[string]string, 0)
	for _, s := range dataRaw {
		for _, dataRawRow := range s.rows {
			// Make sure source is included in row data
			rowData := map[string]string{
				sourceColumnName: s.path,
			}

			// Run any needed transformations on the row data
			for _, step := range k.cfg.RowTransformSteps {
				dataRawRow, err = step.transformFunc(ctx, k.slogger, dataRawRow)
				if err != nil {
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

// getSourceConstraint retrieves any constraints against the `source` column
func getSourceConstraint(queryContext table.QueryContext) *table.ConstraintList {
	sourceConstraint, sourceConstraintExists := queryContext.Constraints[sourceColumnName]
	if sourceConstraintExists {
		return &sourceConstraint
	}
	return nil
}

// checkSourceConstraints validates whether a given `source` matches the given constraints.
func checkSourceConstraints(source string, sourceConstraints *table.ConstraintList) (bool, error) {
	if sourceConstraints == nil {
		return true, nil
	}

	for _, sourceConstraint := range sourceConstraints.Constraints {
		switch sourceConstraint.Operator {
		case table.OperatorEquals:
			if source != sourceConstraint.Expression {
				return false, nil
			}
		case table.OperatorLike:
			// Transform the expression into a regex to test if we have a match.
			likeRegexpStr := regexp.QuoteMeta(sourceConstraint.Expression)
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
			if !r.MatchString(source) {
				return false, nil
			}
		case table.OperatorGlob:
			// Transform the expression into a regex to test if we have a match.
			// Unlike LIKE, GLOB is case-sensitive.
			globRegexpStr := regexp.QuoteMeta(sourceConstraint.Expression)
			// * matches zero or more characters
			globRegexpStr = strings.Replace(globRegexpStr, `\*`, `.*`, -1)
			// ? matches a single character
			globRegexpStr = strings.Replace(globRegexpStr, `\?`, `.`, -1)
			r, err := regexp.Compile(globRegexpStr)
			if err != nil {
				return false, fmt.Errorf("invalid GLOB statement: %w", err)
			}
			if !r.MatchString(source) {
				return false, nil
			}
		case table.OperatorRegexp:
			r, err := regexp.Compile(sourceConstraint.Expression)
			if err != nil {
				return false, fmt.Errorf("invalid regex: %w", err)
			}
			if !r.MatchString(source) {
				return false, nil
			}
		default:
			return false, fmt.Errorf("operator %v not valid source constraint", sourceConstraint.Operator)
		}
	}

	return true, nil
}
