//go:build linux
// +build linux

package nix_env_upgradeable

import (
	"fmt"
	"io"
	"os/exec"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/desktop/runner"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
)

const allowedCharacters = "0123456789-"

type Table struct {
	logger    log.Logger
	tableName string
	execCC    allowedcmd.AllowedCommand
}

func TablePlugin(logger log.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("uid"),
	)

	t := &Table{
		logger:    logger,
		tableName: "kolide_nix_upgradeable",
		execCC:    allowedcmd.NixEnv,
	}

	return table.NewPlugin(t.tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	for _, uid := range tablehelpers.GetConstraints(queryContext, "uid", tablehelpers.WithAllowedCharacters(allowedCharacters)) {
		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			output, err := t.getUserPackages(ctx, uid)
			if err != nil {
				level.Info(t.logger).Log("msg", "failure querying user installed packages", "err", err, "uid", uid)
				continue
			}

			flattenOpts := []dataflatten.FlattenOpts{
				dataflatten.WithLogger(t.logger),
				dataflatten.WithQuery(strings.Split(dataQuery, "/")),
			}

			flattened, err := t.Xml(output, flattenOpts...)
			if err != nil {
				level.Info(t.logger).Log("msg", "failure flattening output", "err", err)
				continue
			}

			rowData := map[string]string{
				"uid": uid,
			}

			results = append(results, dataflattentable.ToMap(flattened, dataQuery, rowData)...)
		}
	}

	return results, nil
}

func (t *Table) getUserPackages(ctx context.Context, uid string) ([]byte, error) {
	cmd, err := t.execCC(ctx, []string{"--query", "--installed", "-c", "--xml"})
	if err != nil {
		return nil, fmt.Errorf("creating nix-env command: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("assigning StdoutPipe for nix-env command: %w", err)
	}

	user_runner := runner.DesktopUsersProcessesRunner.New()
	err := user_runner.runAsUser(ctx, uid, cmd)
	if err != nil {
		return nil, fmt.Errorf("runAsUser nix-env command as user %s: %w", uid, err)
	}

	data, err := io.ReadAll(output)
	if err != nil {
		return nil, fmt.Errorf("ReadAll nix-env output: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("deallocation of nix-env command as user %s: %w", uid, err)
	}

	return data, nil
}
