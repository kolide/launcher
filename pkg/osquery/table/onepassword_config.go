package table

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
	_ "modernc.org/sqlite"
)

var onepasswordDataFiles = map[string][]string{
	"windows": {"AppData/Local/1password/data/1Password10.sqlite"},
	"darwin": {
		"Library/Application Support/1Password 4/Data/B5.sqlite",
		"Library/Group Containers/2BUA8C4S2C.com.agilebits/Library/Application Support/1Password/Data/B5.sqlite",
		"Library/Containers/2BUA8C4S2C.com.agilebits.onepassword-osx-helper/Data/Library/Data/B5.sqlite",
	},
}

func OnePasswordAccounts(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("username"),
		table.TextColumn("user_email"),
		table.TextColumn("team_name"),
		table.TextColumn("server"),
		table.TextColumn("user_first_name"),
		table.TextColumn("user_last_name"),
		table.TextColumn("account_type"),
	}

	o := &onePasswordAccountsTable{
		slogger: slogger.With("table", "kolide_onepassword_accounts"),
	}

	return tablewrapper.New(flags, slogger, "kolide_onepassword_accounts", columns, o.generate)
}

type onePasswordAccountsTable struct {
	slogger *slog.Logger
}

// generate the onepassword account info results given the path to a
// onepassword sqlite DB
func (o *onePasswordAccountsTable) generateForPath(ctx context.Context, fileInfo userFileInfo) ([]map[string]string, error) {
	_, span := traces.StartSpan(ctx, "path", fileInfo.path)
	defer span.End()

	dir, err := agent.MkdirTemp("kolide_onepassword_accounts")
	if err != nil {
		return nil, fmt.Errorf("creating kolide_onepassword_accounts tmp dir: %w", err)
	}
	defer os.RemoveAll(dir) // clean up

	dst := filepath.Join(dir, "tmpfile")
	if err := fsutil.CopyFile(fileInfo.path, dst); err != nil {
		return nil, fmt.Errorf("copying sqlite db to tmp dir: %w", err)
	}

	db, err := sql.Open("sqlite", dst)
	if err != nil {
		return nil, fmt.Errorf("connecting to sqlite db: %w", err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT user_email, team_name, server, user_first_name, user_last_name, account_type FROM accounts")
	if err != nil {
		return nil, fmt.Errorf("query rows from onepassword account configuration db: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			o.slogger.Log(ctx, slog.LevelWarn,
				"closing rows after scanning results",
				"err", err,
			)
		}
		if err := rows.Err(); err != nil {
			o.slogger.Log(ctx, slog.LevelWarn,
				"encountered iteration error",
				"err", err,
			)
		}
	}()

	var results []map[string]string
	for rows.Next() {
		var email, team, server, firstName, lastName, accountType string
		if err := rows.Scan(&email, &team, &server, &firstName, &lastName, &accountType); err != nil {
			return nil, fmt.Errorf("scanning onepassword account configuration db row: %w", err)
		}
		results = append(results, map[string]string{
			"user_email":      email,
			"username":        fileInfo.user,
			"team_name":       team,
			"server":          server,
			"user_first_name": firstName,
			"user_last_name":  lastName,
			"account_type":    accountType,
		})
	}
	return results, nil
}

func (o *onePasswordAccountsTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx, "table_name", "kolide_onepassword_accounts")
	defer span.End()

	var results []map[string]string
	osDataFiles, ok := onepasswordDataFiles[runtime.GOOS]
	if !ok {
		return results, errors.New("No onepasswordDataFiles for this platform")
	}

	for _, dataFilePath := range osDataFiles {
		files, err := findFileInUserDirs(dataFilePath, o.slogger)
		if err != nil {
			o.slogger.Log(ctx, slog.LevelInfo,
				"find 1password sqlite DBs",
				"path", dataFilePath,
				"err", err,
			)
			continue
		}

		for _, file := range files {
			res, err := o.generateForPath(ctx, file)
			if err != nil {
				o.slogger.Log(ctx, slog.LevelInfo,
					"generating onepassword result",
					"path", file.path,
					"err", err,
				)
				continue
			}
			results = append(results, res...)
		}
	}
	return results, nil
}
