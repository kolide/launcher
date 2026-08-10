package table

import (
	"context"
	"debug/macho"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/kolide/launcher/v2/ee/observability"
	"github.com/kolide/launcher/v2/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

func MachoInfo(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("path"),
		table.TextColumn("name"),
		table.TextColumn("cpu"),
	}

	return tablewrapper.New(flags, slogger, "kolide_macho_info", columns, generateMacho,
		tablewrapper.WithDescription("Mach-O binary metadata including app name and CPU architecture. Requires a WHERE path = constraint. Useful for identifying binary architectures (arm64, x86_64) on macOS."),
	)
}

func generateMacho(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	_, span := observability.StartSpan(ctx, "table_name", "kolide_macho_info")
	defer span.End()

	q, ok := queryContext.Constraints["path"]
	if !ok || len(q.Constraints) == 0 {
		return nil, errors.New("The kolide_macho_info table requires that you specify a constraint WHERE path =")
	}
	path := q.Constraints[0].Expression

	f, thinErr := macho.Open(path)
	// macho.Open only handles thin binaries. When it succeeds, return that
	// single architecture before falling back to macho.OpenFat.
	if thinErr == nil {
		defer f.Close()

		return []map[string]string{machoResult(path, f.Cpu.String())}, nil
	}

	fatFile, err := macho.OpenFat(path)
	if err != nil {
		return nil, fmt.Errorf("opening Mach-O binary: %w", thinErr)
	}
	defer fatFile.Close()

	results := make([]map[string]string, 0, len(fatFile.Arches))
	for _, arch := range fatFile.Arches {
		results = append(results, machoResult(path, arch.File.Cpu.String()))
	}

	return results, nil
}

func machoResult(path, cpu string) map[string]string {
	return map[string]string{
		"path": path,
		"name": appFromPath(path),
		"cpu":  cpu,
	}
}

func appFromPath(path string) string {
	parts := strings.SplitSeq(filepath.ToSlash(path), "/")
	for part := range parts {
		if strings.HasSuffix(part, ".app") {
			return part
		}
	}

	return ""
}
