//go:build linux
// +build linux

package nix_env_upgradeable

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
)

const allowedCharacters = "0123456789"

type Table struct {
	slogger *slog.Logger
	execCC  allowedcmd.AllowedCommand
}

func TablePlugin(slogger *slog.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("uid"),
	)

	t := &Table{
		slogger: slogger.With("table", "kolide_nix_upgradeable"),
		execCC:  allowedcmd.NixEnv,
	}

	return table.NewPlugin("kolide_nix_upgradeable", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	uids := tablehelpers.GetConstraints(queryContext, "uid", tablehelpers.WithAllowedCharacters(allowedCharacters))
	if len(uids) < 1 {
		return results, fmt.Errorf("kolide_nix_upgradeable requires at least one user id to be specified")
	}

	for _, uid := range uids {
		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			output, err := t.getUserPackages(ctx, uid)
			if err != nil {
				t.slogger.Log(ctx, slog.LevelInfo,
					"failure querying user installed packages",
					"err", err,
					"target_uid", uid,
				)
				continue
			}

			flattenOpts := []dataflatten.FlattenOpts{
				dataflatten.WithSlogger(t.slogger),
				dataflatten.WithQuery(strings.Split(dataQuery, "/")),
			}

			flattened, err := dataflatten.Xml(output, flattenOpts...)
			if err != nil {
				t.slogger.Log(ctx, slog.LevelInfo,
					"failure flattening output",
					"err", err,
				)
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
	cmd, err := t.execCC(ctx, "--query", "--installed", "-c", "--xml")
	if err != nil {
		return nil, fmt.Errorf("creating nix-env command: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("assigning StdoutPipe for nix-env command: %w", err)
	}

	if err := runAsUser(ctx, uid, cmd); err != nil {
		return nil, fmt.Errorf("runAsUser nix-env command as user %s: %w", uid, err)
	}

	data, err := io.ReadAll(stdout)
	if err != nil {
		return nil, fmt.Errorf("ReadAll nix-env stdout: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("deallocation of nix-env command as user %s: %w", uid, err)
	}

	return data, nil
}

func runAsUser(ctx context.Context, uid string, cmd *exec.Cmd) error {
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("getting current user: %w", err)
	}

	runningUser, err := user.LookupId(uid)
	if err != nil {
		return fmt.Errorf("looking up user with uid %s: %w", uid, err)
	}

	if currentUser.Uid != "0" {
		if currentUser.Uid != runningUser.Uid {
			return fmt.Errorf("current user %s is not root and can't start process for other user %s", currentUser.Uid, uid)
		}

		return cmd.Start()
	}

	runningUserUid, err := strconv.ParseUint(runningUser.Uid, 10, 32)
	if err != nil {
		return fmt.Errorf("converting uid %s to int: %w", runningUser.Uid, err)
	}

	runningUserGid, err := strconv.ParseUint(runningUser.Gid, 10, 32)
	if err != nil {
		return fmt.Errorf("converting gid %s to int: %w", runningUser.Gid, err)
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uint32(runningUserUid),
			Gid: uint32(runningUserGid),
		},
	}

	return cmd.Start()
}
