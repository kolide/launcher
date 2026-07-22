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

const (
	packageInfoTableName = "kolide_pkgutil_package_info"
	packagesTableName    = "kolide_pkgutil_packages"
	rootVolume           = "/"
)

type Pkgutil struct {
	slogger *slog.Logger
}

func PkgutilPackages(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("package_id"),
		table.TextColumn("volume"),
	}

	pkgutilTable := &Pkgutil{
		slogger: slogger.With("table", packagesTableName),
	}

	return tablewrapper.New(flags, slogger, packagesTableName, columns, pkgutilTable.generatePackages,
		tablewrapper.WithDescription("macOS Installer packages queried from the receipt database used by Installer for installed packages. Optionally takes a WHERE volume = constraint to specify the volume to check."),
	)
}

type pkgutilExecutor struct {
	ctx     context.Context // nolint:containedctx
	slogger *slog.Logger
}

func (p *pkgutilExecutor) ExecPackages(volume string) ([]byte, error) {
	return tablehelpers.RunSimple(p.ctx, p.slogger, 10, allowedcmd.Pkgutil, []string{"--volume", volume, "--pkgs"})
}

func (p *pkgutilExecutor) ExecPackageInfo(volume, packageID string) ([]byte, error) {
	return tablehelpers.RunSimple(p.ctx, p.slogger, 10, allowedcmd.Pkgutil, []string{"--volume", volume, fmt.Sprintf("--pkg-info=%s", packageID)})
}

//mockery:generate: true
//mockery:filename: executor.go
//mockery:structname: Executor
type executor interface {
	ExecPackages(volume string) ([]byte, error)
	ExecPackageInfo(volume, packageID string) ([]byte, error)
}

func (t *Pkgutil) generatePackages(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", packagesTableName)
	defer span.End()

	pkgutilExec := &pkgutilExecutor{
		ctx:     ctx,
		slogger: t.slogger,
	}

	return generatePackagesData(ctx, queryContext, pkgutilExec, t.slogger)
}

func generatePackagesData(ctx context.Context, queryContext table.QueryContext, pkgutilExec executor, slogger *slog.Logger) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	results := make([]map[string]string, 0)

	volumes := tablehelpers.GetConstraints(queryContext, "volume", tablehelpers.WithDefaults(rootVolume))
	for _, volume := range volumes {
		// don't fail this if the directory doesn't exist, there won't be any packages anyway
		if _, err := os.Stat(volume); os.IsNotExist(err) {
			continue
		}

		output, err := pkgutilExec.ExecPackages(volume)
		if err != nil {
			// pkgutil returns exit status 1 for no results
			if strings.Contains(err.Error(), "exit status 1") {
				continue
			}

			// log that the binary doesn't exist, but don't return an error
			if os.IsNotExist(errors.Cause(err)) {
				slogger.Log(ctx, slog.LevelWarn,
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

func PkgutilPackageInfo(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("package_id"),
		table.TextColumn("version"),
		table.TextColumn("volume"),
		table.TextColumn("location"),
		table.IntegerColumn("install_time"),
		table.TextColumn("groups"),
	}

	pkgutilTable := &Pkgutil{
		slogger: slogger.With("table", packageInfoTableName),
	}

	return tablewrapper.New(flags, slogger, packageInfoTableName, columns, pkgutilTable.generatePackageInfo,
		tablewrapper.WithDescription("macOS Installer package extended information queried from the receipt database used by Installer for installed packages. Requires a WHERE package_id = constraint to specify the package."),
	)
}

func (t *Pkgutil) generatePackageInfo(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", packageInfoTableName)
	defer span.End()

	pkgutilExec := &pkgutilExecutor{
		ctx:     ctx,
		slogger: t.slogger,
	}

	return generatePackageInfoData(ctx, queryContext, pkgutilExec, t.slogger)
}

func generatePackageInfoData(ctx context.Context, queryContext table.QueryContext, pkgutilExec executor, slogger *slog.Logger) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	results := make([]map[string]string, 0)

	packageIDs := tablehelpers.GetConstraints(queryContext, "package_id")
	if len(packageIDs) == 0 {
		slogger.Log(ctx, slog.LevelError,
			"no package_id provided",
		)
		return nil, fmt.Errorf("The %s table requires that you specify a constraint for WHERE package_id.", packageInfoTableName)
	}

	for _, packageID := range packageIDs {
		for _, volume := range tablehelpers.GetConstraints(queryContext, "volume", tablehelpers.WithDefaults(rootVolume)) {
			// don't fail this if the directory doesn't exist, there won't be any packages anyway
			if _, err := os.Stat(volume); os.IsNotExist(err) {
				continue
			}

			output, err := pkgutilExec.ExecPackageInfo(volume, packageID)
			if err != nil {
				// pkgutil returns exit status 1 when the package is not installed on the volume
				if strings.Contains(err.Error(), "exit status 1") {
					continue
				}

				if os.IsNotExist(errors.Cause(err)) {
					slogger.Log(ctx, slog.LevelWarn,
						"pkgutil binary not found",
						"err", err,
					)
					return nil, nil
				}

				slogger.Log(ctx, slog.LevelError,
					"pkgutil failed",
					"volume", volume,
					"package_id", packageID,
					"err", err,
				)
				continue
			}

			parsed, err := parsePkgInfoOutput(output)
			if err != nil {
				slogger.Log(ctx, slog.LevelError,
					"failed to parse pkgutil output",
					"volume", volume,
					"package_id", packageID,
					"err", err,
				)
				continue
			}

			if parsed["package_id"] == "" {
				continue
			}

			results = append(results, parsed)
		}
	}

	return results, nil
}
