//go:build linux
// +build linux

package xrdb

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
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/osquery/osquery-go/plugin/table"
)

const allowedUsernameCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-."
const allowedDisplayCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789:."

type execer func(ctx context.Context, display, username string, buf *bytes.Buffer) error

type XRDBSettings struct {
	slogger  *slog.Logger
	getBytes execer
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("key"),
		table.TextColumn("value"),
		table.TextColumn("display"),
		table.TextColumn("username"),
	}

	t := &XRDBSettings{
		slogger:  slogger.With("table", "kolide_xrdb"),
		getBytes: execXRDB,
	}

	return tablewrapper.New(flags, slogger, "kolide_xrdb", columns, t.generate)
}

func (t *XRDBSettings) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", "kolide_xrdb")
	defer span.End()

	var results []map[string]string

	users := tablehelpers.GetConstraints(queryContext, "username", tablehelpers.WithAllowedCharacters(allowedUsernameCharacters))
	if len(users) < 1 {
		return results, errors.New("kolide_xrdb requires at least one username to be specified")
	}

	displays := tablehelpers.GetConstraints(queryContext, "display",
		tablehelpers.WithAllowedCharacters(allowedDisplayCharacters),
		tablehelpers.WithDefaults(":0"),
	)
	for _, username := range users {
		for _, display := range displays {
			var output bytes.Buffer

			err := t.getBytes(ctx, display, username, &output)
			if err != nil {
				t.slogger.Log(ctx, slog.LevelInfo,
					"error getting bytes for user",
					"username", username,
					"err", err,
				)
				continue
			}
			user_results := t.parse(ctx, display, username, &output)
			results = append(results, user_results...)
		}
	}

	return results, nil
}

// execXRDB writes the output of running 'xrdb' command into the
// supplied bytes buffer
func execXRDB(ctx context.Context, displayNum, username string, buf *bytes.Buffer) error {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("finding user by username '%s': %w", username, err)
	}

	var stderr bytes.Buffer

	dir, err := agent.MkdirTemp("osq-xrdb")
	if err != nil {
		return fmt.Errorf("mktemp: %w", err)
	}
	defer os.RemoveAll(dir)

	if err := os.Chmod(dir, 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	args := []string{"-display", displayNum, "-global", "-query"}
	// set the HOME cmd so that xrdb is exec'd properly as the new user.
	if err := tablehelpers.Run(ctx, multislogger.NewNopLogger(), 45, allowedcmd.Xrdb, args, buf, &stderr,
		tablehelpers.WithUid(u.Uid),
		tablehelpers.WithAppendEnv("HOME", u.HomeDir),
		tablehelpers.WithDir(dir),
	); err != nil {
		return fmt.Errorf("running xrdb, err is: %s: %w", stderr.String(), err)
	}

	return nil
}

func (t *XRDBSettings) parse(ctx context.Context, display, username string, input io.Reader) []map[string]string {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	var results []map[string]string

	scanner := bufio.NewScanner(input)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			t.slogger.Log(ctx, slog.LevelError,
				"unable to process line, not enough segments",
				"line", line,
			)
			continue
		}
		row := make(map[string]string)
		row["key"] = parts[0]
		row["value"] = strings.TrimSpace(parts[1])
		row["display"] = display
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
