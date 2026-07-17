//go:build darwin

package pkgutil

import (
	"bufio"
	"bytes"
	"context"
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

const rootVolume = "/"

type Table struct {
	slogger *slog.Logger
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("package_id"),
		table.TextColumn("volume"),
	}

	t := &Table{
		slogger: slogger.With("table", "kolide_pkgutil_packages"),
	}

	return tablewrapper.New(flags, slogger, "kolide_pkgutil_packages", columns, t.generate,
		tablewrapper.WithDescription("macOS Installer packages queried from the receipt database used by Installer for installed packages. Optionally takes a WHERE volume = constraint to specify the volume to check."),
	)
}

type pkgutilExecutor struct {
	ctx     context.Context // nolint:containedctx
	slogger *slog.Logger
}

func (p *pkgutilExecutor) Exec(volume string) ([]byte, error) {
	return tablehelpers.RunSimple(p.ctx, p.slogger, 10, allowedcmd.Pkgutil, []string{"--volume", volume, "--pkgs"})
}

//mockery:generate: true
//mockery:filename: executor.go
//mockery:structname: Executor
type executor interface {
	Exec(volume string) ([]byte, error)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", "kolide_pkgutil_packages")
	defer span.End()

	pkgutilExecutor := &pkgutilExecutor{
		ctx:     ctx,
		slogger: t.slogger,
	}

	return generatePkgutilData(ctx, queryContext, pkgutilExecutor, t.slogger)
}

func generatePkgutilData(ctx context.Context, queryContext table.QueryContext, pkgutilExecutor executor, slogger *slog.Logger) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	results := make([]map[string]string, 0)

	volumes := tablehelpers.GetConstraints(queryContext, "volume", tablehelpers.WithDefaults(rootVolume))
	for _, volume := range volumes {
		// don't fail this if the directory doesn't exist, there won't be any packages anyway
		if _, err := os.Stat(volume); os.IsNotExist(err) {
			continue
		}

		output, err := pkgutilExecutor.Exec(volume)
		if err != nil {
			// pkgutil returns exit status 1 for no results
			if strings.Contains(err.Error(), "exit status 1") {
				continue
			}

			// log that the binary doesn't exist, but don't return an error
			if os.IsNotExist(errors.Cause(err)) {
				slogger.Log(ctx, slog.LevelError,
					"pkgutil binary not found",
					"err", err,
				)
				return nil, nil
			}

			slogger.Log(ctx, slog.LevelError,
				"pkgutil failed",
				"volume", volume,
				"err", err,
			)
			return results, nil
		}

		scanner := bufio.NewScanner(bytes.NewReader(output))
		for scanner.Scan() {
			line := scanner.Text()
			results = append(results, map[string]string{
				"package_id": line,
				"volume":     volume,
			})
		}
	}

	return results, nil
}
