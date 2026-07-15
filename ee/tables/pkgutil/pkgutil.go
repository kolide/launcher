//go:build darwin

package pkgutil

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/kolide/launcher/v2/ee/allowedcmd"
	"github.com/kolide/launcher/v2/ee/observability"
	"github.com/kolide/launcher/v2/ee/tables/tablehelpers"
	"github.com/kolide/launcher/v2/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

type Table struct {
	slogger *slog.Logger
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("package_id"),
	}

	t := &Table{
		slogger: slogger.With("table", "kolide_pkgutil_packages"),
	}

	return tablewrapper.New(flags, slogger, "kolide_pkgutil_packages", columns, t.generate,
		tablewrapper.WithDescription("macOS Installer packages and receipts data from `pkgutil --pkgs`. Queries the receipt database used by Installer for installed packages"),
	)
}

type pkgutilExecutor struct {
	ctx     context.Context // nolint:containedctx
	slogger *slog.Logger
}

func (p *pkgutilExecutor) Exec() ([]byte, error) {
	return tablehelpers.RunSimple(p.ctx, p.slogger, 10, allowedcmd.Pkgutil, []string{"--pkgs"})
}

//mockery:generate: true
//mockery:filename: executor.go
//mockery:structname: Executor
type executor interface {
	Exec() ([]byte, error)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", "kolide_pkgutil_packages")
	defer span.End()

	pkgutilExecutor := &pkgutilExecutor{
		ctx:     ctx,
		slogger: t.slogger,
	}

	return generatePkgutilData(ctx, pkgutilExecutor, t.slogger)
}

func generatePkgutilData(ctx context.Context, pkgutilExecutor executor, slogger *slog.Logger) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	output, err := pkgutilExecutor.Exec()
	if err != nil {
		slogger.Log(ctx, slog.LevelInfo,
			"pkgutil failed",
			"err", err,
		)

		// Don't error out if the binary isn't found
		if os.IsNotExist(errors.Cause(err)) {
			return nil, nil
		}
		return nil, fmt.Errorf("calling pkgutil: %w", err)
	}

	results := make([]map[string]string, 0)

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		packageID := strings.TrimSuffix(line, "\n")
		results = append(results, map[string]string{
			"package_id": packageID,
		})
	}

	return results, nil
}
