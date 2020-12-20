// +build darwin

package filevault

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

const fdesetupPath = "/usr/bin/fdesetup"

type Table struct {
	client *osquery.ExtensionManagerClient
	logger log.Logger
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("status"),
	}

	t := &Table{
		client: client,
		logger: logger,
	}

	return table.NewPlugin("kolide_filevault", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	// Read the system's fdesetup configuration
	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, fdesetupPath, "status")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, errors.Wrap(err, "calling fdesetup")
	}
	output := string(stdout.Bytes())
	status := strings.TrimSuffix(output, "\n")
	result := map[string]string{
		"status": status,
	}
	results = append(results, result)

	return results, nil
}
