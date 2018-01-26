package osquery

import (
	"context"
	"debug/macho"

	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

func MachOInfo() *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("path"),
		table.TextColumn("cpu"),
	}

	return table.NewPlugin("kolide_macho_info", columns, generateMacho)
}

func generateMacho(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	q, ok := queryContext.Constraints["path"]
	if !ok || len(q.Constraints) == 0 {
		return nil, errors.New("The kolide_macho_info table requires that you specify a constraint WHERE path =")
	}
	path := q.Constraints[0].Expression
	f, err := macho.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var results []map[string]string
	results = append(results, map[string]string{
		"path": path,
		"cpu":  f.Cpu.String(),
	})
	return results, nil
}
