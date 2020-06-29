// firmwarepasswd is a simple wrapper around the
// `/usr/sbin/firmwarepasswd` tool. This should be considered beta at
// best.

package firmwarepasswd

import (
	"bufio"
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

type Table struct {
	client *osquery.ExtensionManagerClient
	logger log.Logger
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {

	columns := []table.ColumnDefinition{
		table.IntegerColumn("password_enabled"),
		table.TextColumn("mode"),
		table.TextColumn("option_roms_allowed"),
	}

	t := &Table{
		client: client,
		logger: level.NewFilter(logger, level.AllowInfo()),
	}

	return table.NewPlugin("kolide_firmwarepasswd", columns, t.generate)

}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	result := make(map[string]string)

	for _, mode := range []string{"-check", "-mode"} {
		output := new(bytes.Buffer)
		if err := t.runFirmwarepasswd(ctx, mode, output); err != nil {
			level.Info(t.logger).Log(
				"msg", "Error running firmware password",
				"command", mode,
				"err", err,
			)
			continue
		}

		// parse output into our results
		if err := t.parseOutput(output, result); err != nil {
			level.Info(t.logger).Log(
				"msg", "Error running firmware password",
				"command", mode,
				"err", err,
			)
			continue
		}
	}
	return []map[string]string{result}, nil
}

func (t *Table) runFirmwarepasswd(ctx context.Context, subcommand string, output *bytes.Buffer) error {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/usr/sbin/firmwarepasswd", subcommand)

	dir, err := ioutil.TempDir("", "osq-firmwarepasswd")
	if err != nil {
		return errors.Wrap(err, "mktemp")
	}
	defer os.RemoveAll(dir)

	if err := os.Chmod(dir, 0755); err != nil {
		return errors.Wrap(err, "chmod")
	}

	cmd.Dir = dir

	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr

	cmd.Stdout = output

	if err := cmd.Run(); err != nil {
		level.Info(t.logger).Log(
			"msg", "Error running firmwarepasswd",
			"stderr", strings.TrimSpace(stderr.String()),
			"stdout", strings.TrimSpace(output.String()),
			"err", err,
		)
		return errors.Wrap(err, "running osquery")
	}
	return nil
}

func (t *Table) parseOutput(input *bytes.Buffer, row map[string]string) error {
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		switch {
		case strings.HasPrefix(line, "Password Enabled: "):
			value, err := passwordValue(line)
			if err != nil {
				level.Debug(t.logger).Log("msg", "unable to parse password value", "err", err)
				continue
			}
			row["password_enabled"] = value

		case strings.HasPrefix(line, "Mode: "):
			value, err := modeValue(line)
			if err != nil {
				level.Debug(t.logger).Log("msg", "unable to parse mode value", "err", err)
				continue
			}
			row["mode"] = value

		case strings.HasPrefix(line, "Option roms "):
			value, err := optionRomValue(line)
			if err != nil {
				level.Debug(t.logger).Log("msg", "unable to parse option rom value", "err", err)
				continue
			}
			row["option_roms_allowed"] = value
		}
	}

	if err := scanner.Err(); err != nil {
		level.Debug(t.logger).Log("msg", "scanner error", "err", err)
	}

	return nil
}

func optionRomValue(in string) (string, error) {
	switch strings.TrimPrefix(in, "Option roms ") {
	case "not allowed":
		return "0", nil
	case "allowed":
		return "1", nil
	}
	return "", errors.Errorf("Can't tell value from %s", in)
}

func passwordValue(in string) (string, error) {
	components := strings.SplitN(in, ":", 2)
	if len(components) < 2 {
		return "", errors.Errorf("Can't tell value from %s", in)
	}

	t, err := discernValBool(components[1])

	if t {
		return "1", err
	}
	return "0", err
}

func modeValue(in string) (string, error) {
	components := strings.SplitN(in, ":", 2)
	if len(components) < 2 {
		return "", errors.Errorf("Can't tell mode from %s", in)
	}

	return strings.TrimSpace(strings.ToLower(components[1])), nil
}

func discernValBool(in string) (bool, error) {
	switch strings.TrimSpace(strings.ToLower(in)) {
	case "true", "t", "1", "y", "yes":
		return true, nil
	case "false", "f", "0", "n", "no":
		return false, nil
	}

	return false, errors.Errorf("Can't discern boolean from string <%s>", in)
}
