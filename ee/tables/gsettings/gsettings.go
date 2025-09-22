//go:build linux
// +build linux

package gsettings

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/user"
	"strings"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

const allowedCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-."

type gsettingsExecer func(ctx context.Context, slogger *slog.Logger, username string, buf *bytes.Buffer) error

type GsettingsValues struct {
	slogger  *slog.Logger
	getBytes gsettingsExecer
}

// Settings returns a table plugin for querying setting values from the
// gsettings command.
func Settings(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("schema"),
		table.TextColumn("key"),
		table.TextColumn("value"),
		table.TextColumn("username"),
	}

	t := &GsettingsValues{
		slogger:  slogger.With("table", "kolide_gsettings"),
		getBytes: execGsettings,
	}

	return tablewrapper.New(flags, slogger, "kolide_gsettings", columns, t.generate)
}

func (t *GsettingsValues) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", "kolide_gsettings")
	defer span.End()

	var results []map[string]string

	users := tablehelpers.GetConstraints(queryContext, "username", tablehelpers.WithAllowedCharacters(allowedCharacters))
	if len(users) < 1 {
		return results, errors.New("kolide_gsettings requires at least one username to be specified")
	}
	for _, username := range users {
		var output bytes.Buffer

		err := t.getBytes(ctx, t.slogger, username, &output)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"error getting bytes for user",
				"username", username,
				"err", err,
			)
			continue
		}

		user_results := t.parse(ctx, username, &output)
		results = append(results, user_results...)
	}

	return results, nil
}

// execGsettings writes the output of running 'gsettings' command into the
// supplied bytes buffer
func execGsettings(ctx context.Context, slogger *slog.Logger, username string, buf *bytes.Buffer) error {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("finding user by username '%s': %w", username, err)
	}

	dir, err := agent.MkdirTemp("osq-gsettings")
	if err != nil {
		return fmt.Errorf("mktemp: %w", err)
	}
	defer os.RemoveAll(dir)

	// if we don't chmod the dir, we get errors like:
	// 'fork/exec /usr/bin/gsettings: permission denied'
	if err := os.Chmod(dir, 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	var stderr bytes.Buffer

	if err := tablehelpers.Run(ctx, slogger, 2,
		allowedcmd.Gsettings, []string{"list-recursively"}, buf, &stderr,
		tablehelpers.WithUid(u.Uid),
		tablehelpers.WithAppendEnv("HOME", u.HomeDir),
		tablehelpers.WithDir(dir),
	); err != nil {
		return fmt.Errorf("creating gsettings command: %w", err)
	}

	return nil
}

func (t *GsettingsValues) parse(ctx context.Context, username string, input io.Reader) []map[string]string {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	var results []map[string]string

	scanner := bufio.NewScanner(input)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, " ", 3)
		if len(parts) < 3 {
			t.slogger.Log(ctx, slog.LevelError,
				"unable to process line, not enough segments",
				"line", line,
			)
			continue
		}
		row := make(map[string]string)
		row["schema"] = parts[0]
		row["key"] = parts[1]
		row["value"] = parts[2]
		row["username"] = username

		results = append(results, row)
	}

	if err := scanner.Err(); err != nil {
		t.slogger.Log(ctx, slog.LevelDebug,
			"scanner error",
			"err", err,
		)
	}

	return results
}
