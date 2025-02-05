package secureboot

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/efi"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

type Table struct {
	slogger *slog.Logger
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.IntegerColumn("secure_boot"),
		table.IntegerColumn("setup_mode"),
	}

	t := &Table{
		slogger: slogger.With("table", "kolide_secureboot"),
	}

	return tablewrapper.New(flags, slogger, "kolide_secureboot", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx, "table_name", "kolide_secureboot")
	defer span.End()

	sb, err := efi.ReadSecureBoot()
	if err != nil {
		t.slogger.Log(ctx, slog.LevelInfo,
			"unable to read secureboot",
			"err", err,
		)
		return nil, fmt.Errorf("Reading secure_boot from efi: %w", err)
	}

	sm, err := efi.ReadSetupMode()
	if err != nil {
		t.slogger.Log(ctx, slog.LevelInfo,
			"unable to read setupmode",
			"err", err,
		)
		return nil, fmt.Errorf("Reading setup_mode from efi: %w", err)
	}

	row := map[string]string{
		"secure_boot": boolToIntString(sb),
		"setup_mode":  boolToIntString(sm),
	}

	return []map[string]string{row}, nil
}

func boolToIntString(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
