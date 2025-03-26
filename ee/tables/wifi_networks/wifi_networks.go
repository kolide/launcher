//go:build windows
// +build windows

package wifi_networks

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/traces"
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
	slogger  *slog.Logger
	getBytes execer
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := dataflattentable.Columns()

	t := &Table{
		slogger:  slogger.With("table", "kolide_wifi_networks"),
		getBytes: execPwsh(slogger),
	}

	return tablewrapper.New(flags, slogger, "kolide_wifi_networks", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx, "table_name", "kolide_wifi_networks")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var results []map[string]string
	var output bytes.Buffer

	if err := t.getBytes(ctx, &output); err != nil {
		return results, fmt.Errorf("getting raw data: %w", err)
	}
	rows, err := dataflatten.Json(output.Bytes(), dataflatten.WithSlogger(t.slogger))
	if err != nil {
		return results, fmt.Errorf("flattening json output: %w", err)
	}

	return append(results, dataflattentable.ToMap(rows, "", nil)...), nil
}

func execPwsh(slogger *slog.Logger) execer {
	return func(ctx context.Context, buf *bytes.Buffer) error {
		ctx, span := traces.StartSpan(ctx)
		defer span.End()

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
		var stderr bytes.Buffer

		err = tablehelpers.Run(ctx, slogger, 45, allowedcmd.Powershell, args, buf, &stderr, tablehelpers.WithDir(dir))
		errOutput := stderr.String()
		// sometimes the powershell script logs errors to stderr, but returns a
		// successful execution code.
		if err != nil || errOutput != "" {
			// if there is an error, inspect the contents of stdout
			slogger.Log(ctx, slog.LevelDebug,
				"error execing, inspecting stdout contents",
				"stdout", buf.String(),
				"err", err,
			)

			if err == nil {
				err = errors.New("exec succeeded, but emitted to stderr")
			}
			return fmt.Errorf("execing powershell, got: %s: %w", errOutput, err)
		}

		return nil
	}
}
