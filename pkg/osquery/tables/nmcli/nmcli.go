package nmcli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

type Table struct {
	client   *osquery.ExtensionManagerClient
	logger   log.Logger
	getBytes execer
	parser   tablehelpers.OutputParser
}

// TODO: are there other places this lives on various distros/flavors?
const nmcliPath = "/usr/bin/nmcli"

type execer func(ctx context.Context, fields []string) ([]byte, error)

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {

	// columns := []table.ColumnDefinition{
	// 	table.TextColumn("ssid"),
	// 	table.TextColumn("bssid"),
	// 	table.TextColumn("channel"),
	// 	table.TextColumn("rate"),
	// 	table.TextColumn("signal"),
	// 	table.TextColumn("security"),
	// }
	columns := dataflattentable.Columns()
	t := &Table{
		client:   client,
		logger:   logger,
		getBytes: nmcliExecer(logger),
		parser:   newParser(logger),
	}
	return table.NewPlugin("kolide_nmcli", columns, t.generateFlat)
}

func (t *Table) generateFlat(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	fields := []string{"bssid", "ssid", "chan", "rate", "signal", "security"}
	output, err := t.getBytes(ctx, fields)
	if err != nil {
		return results, errors.Wrap(err, "getting output")
	}

	scanner := bufio.NewScanner(bytes.NewBuffer(output))
	scanner.Split(numberOfLinesScanner(len(fields)))
	for scanner.Scan() {
		chunk := scanner.Text()
		rows, err := dataflatten.Ini([]byte(chunk), dataflatten.WithLogger(t.logger))
		if err != nil {
			return results, errors.Wrap(err, "flattening data")
		}
		// should check for blank/null row here
		bssid := ""
		for _, r := range rows {
			if strings.HasSuffix(r.StringPath("/"), "BSSID") {
				bssid = r.Value
				break
			}
		}

		results = append(results, dataflattentable.ToMap(rows, "", map[string]string{"bssid": bssid})...)
	}

	return results, nil
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var rows []map[string]string

	fields := []string{"bssid", "ssid", "chan", "rate", "signal", "security"}
	output, err := t.getBytes(ctx, fields)
	if err != nil {
		return rows, errors.Wrap(err, "getting output")
	}
	scanner := bufio.NewScanner(bytes.NewBuffer(output))
	scanner.Split(numberOfLinesScanner(len(fields)))
	for scanner.Scan() {
		chunk := scanner.Text()
		row := t.parser.Parse(bytes.NewBufferString(chunk))
		// should check for blank/null row here
		rows = append(rows, row)
	}

	return rows, nil
}

func nmcliExecer(logger log.Logger) execer {
	return func(ctx context.Context, fields []string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		var stderr bytes.Buffer
		var stdout bytes.Buffer

		fieldsFlag := fmt.Sprintf("--fields=%s", strings.Join(fields, ","))
		// TODO: nmcli automatically re-scans if the wifi list is > 30 seconds old, so we probably don't need to add "--rescan=yes".
		args := []string{"--mode=multiline", fieldsFlag, "device", "wifi", "list", "--rescan", "yes"}

		cmd := exec.CommandContext(ctx, nmcliPath, args...)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		level.Debug(logger).Log("msg", "calling nmcli", "args", cmd.Args)

		if err := cmd.Run(); err != nil {
			errMsg := string(stderr.Bytes())
			level.Debug(logger).Log("stderr", errMsg)
			return []byte{}, errors.Wrapf(err, "calling nmcli, Got: %s", errMsg)
		}
		return stdout.Bytes(), nil
	}
}

// only use this if you're sure the output doesn't omit lines with empty values.
func numberOfLinesScanner(lines int) bufio.SplitFunc {

	var eol = regexp.MustCompile(`\r?\n`)
	// returns advance,token,err
	// advance is where the scanner should start next time,
	return func(data []byte, atEOF bool) (int, []byte, error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}

		locs := eol.FindAllIndex(data, lines)
		// need to make sure we don't get 'index out of range' errors
		// if this condition is false (and we're not at EOF), we'll end up
		// asking the scanner to try again with more data
		// if len(locs) < lines, then we have incomplete data probably
		if locs != nil && len(locs) > 0 {
			//firstMatch := locs[0]
			lastMatch := locs[len(locs)-1]

			return lastMatch[1], data[0:lastMatch[0]], nil
		}
		// ......

		// If we're at EOF, we have a final, non-terminated line. Return it.
		if atEOF {
			return len(data), data, nil
		}

		// Request more data.
		return 0, nil, nil
	}
}

func newParser(logger log.Logger) tablehelpers.OutputParser {
	return *tablehelpers.NewParser(
		logger,
		[]tablehelpers.Matcher{
			{
				Match:   func(in string) bool { return strings.HasPrefix(in, "SSID") },
				KeyFunc: func(_ string) (string, error) { return "ssid", nil },
				ValFunc: func(in string) (string, error) { return value(in) },
			},
			{
				Match:   func(in string) bool { return strings.HasPrefix(in, "BSSID") },
				KeyFunc: func(_ string) (string, error) { return "bssid", nil },
				ValFunc: func(in string) (string, error) { return value(in) },
			},
			{
				Match: func(in string) bool {
					return strings.HasPrefix(in, "CHAN")
				},
				KeyFunc: func(_ string) (string, error) { return "channel", nil },
				ValFunc: func(in string) (string, error) { return value(in) },
			},
			{
				Match:   func(in string) bool { return strings.HasPrefix(in, "RATE") },
				KeyFunc: func(_ string) (string, error) { return "rate", nil },
				ValFunc: func(in string) (string, error) { return value(in) },
			},
			{
				Match:   func(in string) bool { return strings.HasPrefix(in, "SIGNAL") },
				KeyFunc: func(_ string) (string, error) { return "signal", nil },
				ValFunc: func(in string) (string, error) { return value(in) }, // TODO: Convert this to rssi
			},
			{
				Match:   func(in string) bool { return strings.HasPrefix(in, "SECURITY") },
				KeyFunc: func(_ string) (string, error) { return "security", nil },
				ValFunc: func(in string) (string, error) { return value(in) },
			},
		},
	)
}

func value(in string) (string, error) {
	components := strings.SplitN(in, ":", 2)
	if len(components) < 2 {
		return "", nil
	}

	return strings.TrimSpace(components[1]), nil
}
