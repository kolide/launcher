//go:build windows
// +build windows

package wifi_networks

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/allowedcmd"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/osquery/osquery-go/plugin/table"
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
	logger   log.Logger
	getBytes execer
}

func TablePlugin(logger log.Logger) *table.Plugin {
	columns := dataflattentable.Columns()

	t := &Table{
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
		return results, fmt.Errorf("getting raw data: %w", err)
	}
	rows, err := dataflatten.Json(output.Bytes(), dataflatten.WithLogger(t.logger))
	if err != nil {
		return results, fmt.Errorf("flattening json output: %w", err)
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
		dir, err := agent.MkdirTemp("nativewifi")
		if err != nil {
			return fmt.Errorf("creating nativewifi tmp dir: %w", err)
		}
		defer os.RemoveAll(dir)

		outputFile := filepath.Join(dir, "nativewificode.cs")
		if err := os.WriteFile(outputFile, nativeCode, 0755); err != nil {
			return fmt.Errorf("writing native wifi code: %w", err)
		}

		args := append([]string{"-NoProfile", "-NonInteractive"}, string(pwshScript))
		cmd, err := allowedcmd.Powershell(ctx, args...)
		if err != nil {
			return fmt.Errorf("creating powershell command: %w", err)
		}
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
				err = fmt.Errorf("exec succeeded, but emitted to stderr")
			}
			return fmt.Errorf("execing powershell, got: %s: %w", errOutput, err)
		}

		return nil
	}
}
