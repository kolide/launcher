package nmcli

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

type Table struct {
	client   *osquery.ExtensionManagerClient
	logger   log.Logger
	getBytes execer
}

// TODO: are there other places this lives on various distros/flavors?
const nmcliPath = "/usr/bin/nmcli"

type execer func(ctx context.Context) ([]byte, error)

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := dataflattentable.Columns()
	t := &Table{
		client:   client,
		logger:   logger,
		getBytes: nmcliExecer(logger),
	}
	return table.NewPlugin("kolide_nmcli", columns, t.generateDupeSplitStrategy)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string
	output, err := t.getBytes(ctx)
	if err != nil {
		return results, errors.Wrap(err, "getting output")
	}
	scanner := bufio.NewScanner(bytes.NewBuffer(output))
	scanner.Split(onlyDashesScanner)
	for scanner.Scan() {
		chunk := scanner.Bytes()
		rows, err := dataflatten.StringDelimited(chunk, ":", dataflatten.WithLogger(t.logger))
		if err != nil {
			return results, errors.Wrap(err, "flattening nmcli output")
		}

		if len(rows) > 0 {
			bssid := ""
			for _, r := range rows {
				if strings.HasSuffix(r.StringPath("/"), "BSSID") {
					bssid = r.Value
				}
			}
			extraData := map[string]string{"bssid": bssid}
			results = append(results, dataflattentable.ToMap(rows, "", extraData)...)
		}
	}
	return results, nil
}

func (t *Table) generateDupeSplitStrategy(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string
	output, err := t.getBytes(ctx)
	if err != nil {
		return results, errors.Wrap(err, "getting output")
	}
	rows, err := dataflatten.StringDelimitedUnseparated(output, ":", t.logger, dataflatten.WithLogger(t.logger))
	if err != nil {
		return results, errors.Wrap(err, "flattening nmcli output")
	}

	return append(results, dataflattentable.ToMap(rows, "", nil)...), nil
}

func nmcliExecer(logger log.Logger) execer {
	return func(ctx context.Context) ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		var stderr bytes.Buffer
		var stdout bytes.Buffer

		// --pretty will insert a line of dashes ('-') as a record seperator
		// when used with --mode=multiline
		//
		//args := []string{"--mode=multiline", "--pretty", "--fields=all", "device", "wifi", "list", "--rescan", "yes"}
		args := []string{"--mode=multiline", "--fields=all", "device", "wifi", "list", "--rescan", "auto"}

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

// onlyDashesScanner returns tokens delimited by lines that only contain dashes,
// but may contain trailing whitespace
func onlyDashesScanner(data []byte, atEOF bool) (int, []byte, error) {
	var onlyDashes = regexp.MustCompile(`\r?\n-+[\s]*\r?\n`)
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	loc := onlyDashes.FindIndex(data)
	if loc != nil && loc[0] > 0 {
		return loc[1], data[0:loc[0]], nil
	}

	if atEOF {
		return len(data), data, nil
	}

	// Request more data.
	return 0, nil, nil
}
