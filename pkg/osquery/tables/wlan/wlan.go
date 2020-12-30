// +build windows

package wlan

import (
	"context"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

const allowedCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type Table struct {
	client    *osquery.ExtensionManagerClient
	logger    log.Logger
	tableName string
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("output"),
	}

	t := &Table{
		client:    client,
		logger:    logger,
		tableName: "kolide_wlan",
	}

	return table.NewPlugin(t.tableName, columns, t.generate)
}

const netshCmd = "netsh"

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var results []map[string]string

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	args := []string{"wlan", "show", "networks", "mode=Bssid"}

	cmd := exec.CommandContext(ctx, netshCmd, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, errors.Wrapf(err, "calling netsh wlan. Got: %s", stderr.String())
	}

	scanner := bufio.NewScanner(strings.NewReader(stdout))
	for scanner.Scan() {
		row = map[string]string{"output": scanner.Text()}
		results = append(results, row)
	}

	return results, nil
}
