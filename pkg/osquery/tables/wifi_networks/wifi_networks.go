package wifi_networks

import (
	"bufio"
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/osquery/tables/wifi_networks/internal"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

//go:generate go-bindata -nometadata -nocompress -pkg internal -o internal/assets.go internal/assets/

const netshCmd = "netsh"

var preludeInterfaceLineRegex = regexp.MustCompile(`Interface name :`)
var preludeCountLineRegex = regexp.MustCompile(`There are [0-9]+ networks currently available`)

type execer func(ctx context.Context, buf *bytes.Buffer) error

type WlanTable struct {
	client    *osquery.ExtensionManagerClient
	logger    log.Logger
	tableName string
	getBytes  execer
	parser    *OutputParser
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("name"),
		table.IntegerColumn("signal_strength_percentage"),
		table.TextColumn("bssid"),
		table.TextColumn("rssi"),
		// table.TextColumn("channel"), // TODO: figure out how to find this from nativewifi
	}

	parser := buildParser(logger)
	t := &WlanTable{
		client:    client,
		logger:    logger,
		tableName: "kolide_wifi_networks",
		parser:    parser,
		getBytes:  execPwsh(logger),
	}

	return table.NewPlugin(t.tableName, columns, t.generate)
}

func (t *WlanTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var results []map[string]string
	var output bytes.Buffer

	err := t.getBytes(ctx, &output)
	if err != nil {
		return results, err
	}
	scanner := bufio.NewScanner(&output)
	scanner.Split(blankLineSplitter)
	for scanner.Scan() {
		chunk := scanner.Text()
		row := t.parser.Parse(bytes.NewBufferString(chunk))
		// check for blank rows
		if row != nil && len(row) > 0 {
			results = append(results, row)
		}
	}

	if err := scanner.Err(); err != nil {
		level.Debug(t.logger).Log("msg", "scanner error", "err", err)
	}

	return results, nil
}

func execPwsh(logger log.Logger) execer {
	return func(ctx context.Context, buf *bytes.Buffer) error {
		// MS requires interfaces to complete network scans in <4 seconds
		// give a bit more time for everything else to run.
		ctx, cancel := context.WithTimeout(ctx, 4500*time.Millisecond)
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
		nativeCodeFile, err := os.Create(outputFile)
		if err != nil {
			return errors.Wrap(err, "creating file for native wifi code")
		}

		nativeCode, err := internal.AssetString("internal/assets/nativewifi.cs")
		if err != nil {
			return errors.Wrapf(err, "failed to get asset named $s", "internal/assets/nativewifi.cs")
		}
		_, err = nativeCodeFile.WriteString(nativeCode)
		if err != nil {
			return errors.Wrap(err, "writing native code file")
		}
		if err := nativeCodeFile.Close(); err != nil {
			return errors.Wrap(err, "closing native code file")
		}

		pwsh, err := exec.LookPath("powershell.exe")
		if err != nil {
			return errors.Wrap(err, "finding powershell.exe path")
		}
		psScript, err := internal.AssetString("internal/assets/get-networks.ps1")
		if err != nil {
			return errors.Wrapf(err, "failed to get asset named $s", "internal/assets/get-networks.ps1")
		}
		args := append([]string{"-NoProfile", "-NonInteractive"}, psScript)
		cmd := exec.CommandContext(ctx, pwsh, args...)
		cmd.Dir = dir
		var stderr bytes.Buffer
		cmd.Stdout = buf
		cmd.Stderr = &stderr

		err = cmd.Run()
		errOutput := stderr.String()
		// sometimes the powershell script logs errors to stderr, but returns a
		// successful execution code. It's helpful to log the output of stderr
		// in this case.
		if err != nil || errOutput != "" {
			level.Debug(logger).Log("stderr", errOutput)
		}

		return nil
	}
}

func buildParser(logger log.Logger) *OutputParser {
	return NewParser(logger,
		[]Matcher{
			{
				Match:   func(in string) bool { return hasTrimmedPrefix(in, "SSID") },
				KeyFunc: func(_ string) (string, error) { return "name", nil },
				ValFunc: func(in string) (string, error) { return extractTableValue(in) },
			},
			{
				Match:   func(in string) bool { return hasTrimmedPrefix(in, "rssi") },
				KeyFunc: func(_ string) (string, error) { return "rssi", nil },
				ValFunc: func(in string) (string, error) { return extractTableValue(in) },
			},
			{
				Match:   func(in string) bool { return hasTrimmedPrefix(in, "linkQuality") },
				KeyFunc: func(_ string) (string, error) { return "signal_strength_percentage", nil },
				ValFunc: func(in string) (string, error) { return extractTableValue(in) },
			},
			{
				Match:   func(in string) bool { return hasTrimmedPrefix(in, "BSSID") },
				KeyFunc: func(_ string) (string, error) { return "bssid", nil },
				ValFunc: func(in string) (string, error) {
					rawval, err := extractTableValue(in)
					if err != nil {
						return rawval, err
					}
					return strings.ReplaceAll(rawval, "-", ":"), nil
				},
			},
		})
}

func extractTableValue(input string) (string, error) {
	// lines usually look something like:
	//   Authentication       : WPA2-Personal
	parts := strings.SplitN(strings.TrimSpace(input), ":", 2)
	if len(parts) < 2 {
		return "", errors.Errorf("unable to determine value from %s", input)
	}
	return strings.TrimSpace(parts[1]), nil
}

func hasTrimmedPrefix(s, prefix string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), prefix)
}
