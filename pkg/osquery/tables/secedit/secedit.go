// +build windows

package secedit

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/go-ini/ini"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

type Table struct {
	client *osquery.ExtensionManagerClient
	logger log.Logger
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {

	columns := []table.ColumnDefinition{
		table.TextColumn("area"),
		table.TextColumn("section"),
		table.TextColumn("key"),
		table.TextColumn("value"),
	}

	t := &Table{
		client: client,
		logger: logger,
	}

	return table.NewPlugin("kolide_secedit", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	areaQ, ok := queryContext.Constraints["area"]
	if !ok || len(areaQ.Constraints) == 0 {
		return nil, errors.New("The kolide_secedit table requires an area (ex: SecurityPolicy")
	}

	for _, areaConstraint := range areaQ.Constraints {
		area := areaConstraint.Expression

		secEditResults, err := t.execSecedit(ctx, area)
		if err != nil {
			continue
		}

		iniFile, err := ini.Load(secEditResults)
		if err != nil {
			level.Info(t.logger).Log("msg", "ini parsing of secedit output failed", "err", err)
			continue
		}

		for _, section := range iniFile.Sections() {
			for _, key := range section.Keys() {
				result := map[string]string{
					"area":    area,
					"section": section.Name(),
					"key":     key.Name(),
					"value":   key.Value(),
				}
				results = append(results, result)
			}
		}
	}
	return results, nil
}

func (t *Table) execSecedit(ctx context.Context, area string) ([]byte, error) {
	// The secedit.exe binary does not support outputting the data we need to stdout
	// Instead we create a tmp directory and pass it to secedit to write the data we need
	// in INI format.
	dir, err := ioutil.TempDir("", "kolide_secedit_config")
	if err != nil {
		return nil, errors.Wrap(err, "creating kolide_secedit_config tmp dir")
	}
	defer os.RemoveAll(dir)

	dst := filepath.Join(dir, "tmpfile")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "secedit", "/export", "/areas", area, "/cfg", dst)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	level.Debug(t.logger).Log("msg", "calling secedit", "args", cmd.Args)

	if err := cmd.Run(); err != nil {
		return nil, errors.Wrapf(err, "calling secedit. Got: %s", string(stderr.Bytes()))
	}

	file, err := os.Open(dst)
	if err != nil {
		return nil, errors.Wrapf(err, "error opening secedit output file: %s", dst)
	}
	defer file.Close()

	// By default, secedit outputs files encoded in UTF16 Little Endian. Sadly the Go INI parser
	// cannot read this format by default, therefore we decode the bytes into UTF-8
	rd := transform.NewReader(file, unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder())
	data, err := ioutil.ReadAll(rd)
	if err != nil {
		return nil, errors.Wrap(err, "cannot read file")
	}

	return data, nil
}
