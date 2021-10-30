//go:build windows
// +build windows

package wifi_networks

import (
	"bytes"
	"context"
	_ "embed"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

// the c# .Net code in the asset below came from
// https://github.com/metageek-llc/ManagedWifi/, and has been modified
// slightly to support polling for scan completion. the powershell
// script initially came from
// https://jordanmills.wordpress.com/2014/01/12/updated-get-bssid-ps1-programmatic-access-to-available-wireless-networks/
// with some slight modifications
//
//go:embed assets/nativewifi.cs
var nativeCode []byte

//go:embed assets/get-networks.ps1
var pwshScript []byte

type execer func(ctx context.Context, buf *bytes.Buffer) error

type Table struct {
	client   *osquery.ExtensionManagerClient
	logger   log.Logger
	getBytes execer
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := dataflattentable.Columns()

	t := &Table{
		client:   client,
		logger:   logger,
		getBytes: execPwsh(logger),
	}

	return table.NewPlugin("kolide_wifi_networks", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var results []map[string]string
	var output bytes.Buffer

	if err := t.getBytes(ctx, &output); err != nil {
		return results, errors.Wrap(err, "getting raw data")
	}
	rows, err := dataflatten.Json(output.Bytes(), dataflatten.WithLogger(t.logger))
	if err != nil {
		return results, errors.Wrap(err, "flattening json output")
	}

	return append(results, dataflattentable.ToMap(rows, "", nil)...), nil
}

func execPwsh(logger log.Logger) execer {
	return func(ctx context.Context, buf *bytes.Buffer) error {
		// MS requires interfaces to complete network scans in <4 seconds, but
		// that appears not to be consistent
		ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()

		// write the c# code to a file, so the powershell script can load it
		// from there. This works around a size limit on args passed to
		// powershell.exe
		dir, err := ioutil.TempDir("", "nativewifi")
		if err != nil {
			return errors.Wrap(err, "creating nativewifi tmp dir")
		}
		defer os.RemoveAll(dir)

		outputFile := filepath.Join(dir, "nativewificode.cs")
		if err := os.WriteFile(outputFile, nativeCode, 0755); err != nil {
			return errors.Wrap(err, "writing native wifi code")
		}

		pwsh, err := exec.LookPath("powershell.exe")
		if err != nil {
			return errors.Wrap(err, "finding powershell.exe path")
		}

		args := append([]string{"-NoProfile", "-NonInteractive"}, string(pwshScript))
		cmd := exec.CommandContext(ctx, pwsh, args...)
		cmd.Dir = dir
		var stderr bytes.Buffer
		cmd.Stdout = buf
		cmd.Stderr = &stderr

		err = cmd.Run()
		errOutput := stderr.String()
		// sometimes the powershell script logs errors to stderr, but returns a
		// successful execution code.
		if err != nil || errOutput != "" {
			// if there is an error, inspect the contents of stdout
			level.Debug(logger).Log("msg", "error execing, inspecting stdout contents", "stdout", buf.String())

			if err == nil {
				err = errors.Errorf("exec succeeded, but emitted to stderr")
			}
			return errors.Wrapf(err, "execing powershell, got: %s", errOutput)
		}

		return nil
	}
}
