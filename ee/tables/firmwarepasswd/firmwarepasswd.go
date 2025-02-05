//go:build darwin
// +build darwin

// firmwarepasswd is a simple wrapper around the
// `/usr/sbin/firmwarepasswd` tool. This should be considered beta at
// best. It serves a bit as a pattern for future exec work.

package firmwarepasswd

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

type Table struct {
	slogger *slog.Logger
	parser  *OutputParser
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.IntegerColumn("option_roms_allowed"),
		table.IntegerColumn("password_enabled"),
		table.TextColumn("mode"),
	}

	t := New(slogger.With("table", "kolide_firmwarepasswd"))

	return tablewrapper.New(flags, slogger, "kolide_firmwarepasswd", columns, t.generate)

}

func New(slogger *slog.Logger) *Table {
	parser := NewParser(slogger,
		[]Matcher{
			{
				Match:   func(in string) bool { return strings.HasPrefix(in, "Password Enabled: ") },
				KeyFunc: func(_ string) (string, error) { return "password_enabled", nil },
				ValFunc: func(in string) (string, error) { return passwordValue(in) },
			},
			{
				Match:   func(in string) bool { return strings.HasPrefix(in, "Mode: ") },
				KeyFunc: func(_ string) (string, error) { return "mode", nil },
				ValFunc: func(in string) (string, error) { return modeValue(in) },
			},
			{
				Match:   func(in string) bool { return strings.HasPrefix(in, "Option roms ") },
				KeyFunc: func(_ string) (string, error) { return "option_roms_allowed", nil },
				ValFunc: func(in string) (string, error) { return optionRomValue(in) },
			},
		})

	return &Table{
		slogger: slogger,
		parser:  parser,
	}

}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx, "table_name", "kolide_firmwarepasswd")
	defer span.End()

	result := make(map[string]string)

	for _, mode := range []string{"-check", "-mode"} {
		output := new(bytes.Buffer)
		if err := t.runFirmwarepasswd(ctx, mode, output); err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"error running firmware password",
				"command", mode,
				"err", err,
			)
			continue
		}

		// Merge resulting matches
		for _, row := range t.parser.Parse(output) {
			for k, v := range row {
				result[k] = v
			}
		}
	}
	return []map[string]string{result}, nil
}

func (t *Table) runFirmwarepasswd(ctx context.Context, subcommand string, output *bytes.Buffer) error {
	ctx, span := traces.StartSpan(ctx, "subcommand", subcommand)
	defer span.End()

	dir, err := agent.MkdirTemp("osq-firmwarepasswd")
	if err != nil {
		return fmt.Errorf("mktemp: %w", err)
	}
	defer os.RemoveAll(dir)

	if err := os.Chmod(dir, 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	stderr := new(bytes.Buffer)

	if err := tablehelpers.Run(ctx, t.slogger, 1, allowedcmd.Firmwarepasswd, []string{subcommand}, output, stderr, tablehelpers.WithDir(dir)); err != nil {
		t.slogger.Log(ctx, slog.LevelDebug,
			"error running firmwarepasswd",
			"stderr", strings.TrimSpace(stderr.String()),
			"stdout", strings.TrimSpace(output.String()),
			"err", err,
		)
		return fmt.Errorf("running firmwarepasswd: %w", err)
	}
	return nil
}

func modeValue(in string) (string, error) {
	components := strings.SplitN(in, ":", 2)
	if len(components) < 2 {
		return "", fmt.Errorf("Can't tell mode from %s", in)
	}

	return strings.TrimSpace(strings.ToLower(components[1])), nil
}

func passwordValue(in string) (string, error) {
	components := strings.SplitN(in, ":", 2)
	if len(components) < 2 {
		return "", fmt.Errorf("Can't tell value from %s", in)
	}

	t, err := discernValBool(components[1])

	if t {
		return "1", err
	}
	return "0", err
}

func optionRomValue(in string) (string, error) {
	switch strings.TrimPrefix(in, "Option roms ") {
	case "not allowed":
		return "0", nil
	case "allowed":
		return "1", nil
	}
	return "", fmt.Errorf("Can't tell value from %s", in)
}

func discernValBool(in string) (bool, error) {
	switch strings.TrimSpace(strings.ToLower(in)) {
	case "true", "t", "1", "y", "yes":
		return true, nil
	case "false", "f", "0", "n", "no":
		return false, nil
	}

	return false, fmt.Errorf("Can't discern boolean from string <%s>", in)
}
