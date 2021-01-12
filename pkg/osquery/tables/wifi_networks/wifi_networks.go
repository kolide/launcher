package wifi_networks

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

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

	parser := buildParserFull(logger)
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

func (t *WlanTable) generateNetsh(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var results []map[string]string
	var output bytes.Buffer

	err := t.getBytes(ctx, &output)
	if err != nil {
		return results, errors.Wrap(err, "getting raw wlan output")
	}

	scanner := bufio.NewScanner(&output)
	scanner.Split(blankLineSplitter)
	for scanner.Scan() {
		chunk := scanner.Text()
		if len(preludeInterfaceLineRegex.FindStringSubmatch(chunk)) > 0 {
			continue
		}
		if len(preludeCountLineRegex.FindStringSubmatch(chunk)) > 0 {
			continue
		}

		row := t.parser.Parse(bytes.NewBufferString(chunk))
		if row != nil {
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

		_, err = nativeCodeFile.WriteString(nativeWiFiCode)
		if err != nil {
			return errors.Wrap(err, "writing native code file")
		}
		if err := nativeCodeFile.Close(); err != nil {
			return errors.Wrap(err, "closing native code file")
		}

		tmpl, err := template.New("command").Parse(getBSSIDCommandTemplate)
		if err != nil {
			return errors.Wrap(err, "parsing template")
		}
		commandOpts := struct {
			NativeCodePath string
		}{NativeCodePath: nativeCodeFile.Name()}
		var command bytes.Buffer
		if err := tmpl.ExecuteTemplate(&command, "command", commandOpts); err != nil {
			return errors.Wrap(err, "executing template")
		}

		pwsh, err := exec.LookPath("powershell.exe")
		if err != nil {
			return errors.Wrap(err, "finding powershell.exe path")
		}

		args := append([]string{"-NoProfile", "-NonInteractive"}, command.String())
		cmd := exec.CommandContext(ctx, pwsh, args...)

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

func execCmd(ctx context.Context, buf *bytes.Buffer) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var stderr bytes.Buffer

	args := []string{"wlan", "show", "networks", "mode=Bssid"}

	cmd := exec.CommandContext(ctx, netshCmd, args...)
	cmd.Stdout = buf
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "calling netsh wlan, got: %s", stderr.String())
	}

	return nil
}

func buildParser(logger log.Logger) *OutputParser {
	return NewParser(logger,
		[]Matcher{
			{
				Match: func(in string) bool {
					m, err := regexp.MatchString("^SSID [0-9]+", in)
					if err != nil {
						level.Debug(logger).Log("msg", "unable to match regexp", "err", err)
						return false
					}
					return m
				},
				KeyFunc: func(_ string) (string, error) { return "name", nil },
				ValFunc: extractTableValue,
			},
			{
				Match:   func(in string) bool { return hasTrimmedPrefix(in, "Authentication") },
				KeyFunc: func(_ string) (string, error) { return "authentication", nil },
				ValFunc: extractTableValue,
			},
			{
				Match:   func(in string) bool { return hasTrimmedPrefix(in, "Signal") },
				KeyFunc: func(_ string) (string, error) { return "signal_strength_percentage", nil },
				ValFunc: func(in string) (string, error) {
					val, err := extractTableValue(in)
					if err != nil {
						return val, err
					}
					return strings.TrimSuffix(val, "%"), nil
				},
			},
			{
				Match:   func(in string) bool { return hasTrimmedPrefix(in, "Signal") },
				KeyFunc: func(_ string) (string, error) { return "rssi", nil },
				ValFunc: func(in string) (string, error) {
					val, err := extractTableValue(in)
					if err != nil {
						return val, err
					}
					intVal, err := strconv.Atoi(strings.TrimSuffix(val, "%"))
					if err != nil {
						return "", errors.Wrap(err, "converting string to int")
					}
					// a signal strength value of 0 implies an RSSI of -100dbm a
					// signal strength value of 100 implies an RSSI of -50dbm We
					// can interpolate the values in between. This strategy was
					// suggested by code comments from here:
					// https://github.com/metageek-llc/ManagedWifi
					// and, empirically, seems to work out.
					rssi := (-100 + (intVal / 2))

					return fmt.Sprintf("%d", rssi), nil
				},
			},
			{
				Match:   func(in string) bool { return hasTrimmedPrefix(in, "BSSID") },
				KeyFunc: func(_ string) (string, error) { return "bssid", nil },
				ValFunc: extractTableValue,
			},
			{
				Match:   func(in string) bool { return hasTrimmedPrefix(in, "Radio type") },
				KeyFunc: func(_ string) (string, error) { return "radio_type", nil },
				ValFunc: extractTableValue,
			},
			{
				Match:   func(in string) bool { return hasTrimmedPrefix(in, "Channel") },
				KeyFunc: func(_ string) (string, error) { return "channel", nil },
				ValFunc: extractTableValue,
			},
		})
}

func buildParserFull(logger log.Logger) *OutputParser {
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
