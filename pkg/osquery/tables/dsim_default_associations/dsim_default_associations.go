// +build windows

package dsim_default_associations

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
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

const dismCmd = "dism.exe"

type Table struct {
	client *osquery.ExtensionManagerClient
	logger log.Logger
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {

	columns := dataflattentable.Columns()

	t := &Table{
		client: client,
		logger: logger,
	}

	return table.NewPlugin("kolide_dsim_default_associations", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	dismResults, err := t.execDism(ctx)
	if err != nil {
		level.Info(t.logger).Log("msg", "dism failed", "err", err)
		return results, err
	}

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		flattenOpts := []dataflatten.FlattenOpts{
			dataflatten.WithLogger(t.logger),
			dataflatten.WithQuery(strings.Split(dataQuery, "/")),
		}

		rows, err := dataflatten.Xml(dismResults, flattenOpts...)
		if err != nil {
			level.Info(t.logger).Log("msg", "flatten failed", "err", err)
			continue
		}

		results = append(results, dataflattentable.ToMap(rows, dataQuery, nil)...)
	}

	return results, nil
}

func (t *Table) execDism(ctx context.Context) ([]byte, error) {
	// dism.exe outputs xml, but with weird intermingled status. So
	// instead, we dump it to a temp file.
	dir, err := ioutil.TempDir("", "kolide_dism")
	if err != nil {
		return nil, errors.Wrap(err, "creating kolide_dism tmp dir")
	}
	defer os.RemoveAll(dir)

	dstFile := "associations.xml"
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	args := []string{"/online", "/Export-DefaultAppAssociations:" + dstFile}

	cmd := exec.CommandContext(ctx, dismCmd, args...)
	cmd.Dir = dir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	level.Debug(t.logger).Log("msg", "calling dism", "args", cmd.Args)

	if err := cmd.Run(); err != nil {
		return nil, errors.Wrapf(err, "calling dism. Got: %s", stderr.String())
	}

	data, err := ioutil.ReadFile(filepath.Join(dir, dstFile))
	if err != nil {
		return nil, errors.Wrapf(err, "error reading dism output file: %s", err)
	}

	return data, nil
}
