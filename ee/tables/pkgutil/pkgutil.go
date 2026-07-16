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

	volumes := tablehelpers.GetConstraints(queryContext, "volume")
	// the volume constraint is optional
	// when absent, query the root volume and return early
	if len(volumes) < 1 {
		output, err := pkgutilExecutor.Exec(rootVolume)
		if err != nil {
			slogger.Log(ctx, slog.LevelInfo,
				"pkgutil failed",
				"err", err,
			)

			// pkgutil returns exit status 1 for no results
			if strings.Contains(err.Error(), "exit status 1") {
				return nil, nil
			}

			if os.IsNotExist(errors.Cause(err)) {
				return nil, nil
			}
			return nil, fmt.Errorf("calling pkgutil: %w", err)
		}

		scanner := bufio.NewScanner(bytes.NewReader(output))
		for scanner.Scan() {
			line := scanner.Text()
			results = append(results, map[string]string{
				"package_id": line,
				"volume":     rootVolume,
			})
		}

		return results, nil
	}

	for _, volume := range volumes {
		output, err := pkgutilExecutor.Exec(volume)
		if err != nil {
			slogger.Log(ctx, slog.LevelInfo,
				"pkgutil failed",
				"err", err,
			)

			// pkgutil returns exit status 1 for no results
			if strings.Contains(err.Error(), "exit status 1") {
				slogger.Log(ctx, slog.LevelInfo,
					"pkgutil returned no results for volume",
					"volume", volume,
				)
				continue
			}

			if os.IsNotExist(errors.Cause(err)) {
				return nil, nil
			}
			return nil, fmt.Errorf("calling pkgutil: %w", err)
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
