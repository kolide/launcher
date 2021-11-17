package secureboot

import (
	"context"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/efi"
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

type Table struct {
	logger log.Logger
}

func TablePlugin(_client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.IntegerColumn("secure_boot"),
		table.IntegerColumn("setup_mode"),
	}

	t := &Table{
		logger: logger,
	}

	return table.NewPlugin("kolide_secureboot", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {

	sb, err := efi.ReadSecureBoot()
	if err != nil {
		level.Info(t.logger).Log("msg", "Unable to read secureboot", "err", err)
		return nil, errors.Wrap(err, "Reading secure_boot from efi")
	}

	sm, err := efi.ReadSetupMode()
	if err != nil {
		level.Info(t.logger).Log("msg", "Unable to read setupmode", "err", err)
		return nil, errors.Wrap(err, "Reading setup_mode from efi")
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
