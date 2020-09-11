package table

import (
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"

	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

type Table struct {
	client *osquery.ExtensionManagerClient
	logger log.Logger
}

func NetAccounts(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	t := &Table{
		client: client,
		logger: logger,
	}
	columns := []table.ColumnDefinition{
		table.IntegerColumn("force_user_logoff"),
		table.IntegerColumn("min_password_age"),
		table.IntegerColumn("max_password_age"),
		table.IntegerColumn("min_password_length"),
		table.IntegerColumn("length_password_history_maintained"),
		table.IntegerColumn("lockout_threshold"),
		table.IntegerColumn("lockout_duration"),
		table.IntegerColumn("lockout_observation_window"),
		table.TextColumn("computer_role"),
	}

	return table.NewPlugin("kolide_net_accounts", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string
	var forceUserLogoff, minPasswordAge, maxPasswordAge, minPasswordLength, lengthPasswordHistoryMaintained, lockoutThreshold, lockoutDuration, lockoutObservationWindow, computerRole string

	configOutput, err := t.execNetAccounts(ctx)
	if err != nil {
		level.Info(t.logger).Log("msg", "net accounts failed", "err", err)
		return nil, err
	}

	configSplit := strings.Split(configOutput, "\n")

	if len(configSplit) == 12 {
		forceUserLogoff = t.splitAndReadConfigLine(configSplit[0], false)
		minPasswordAge = t.splitAndReadConfigLine(configSplit[1], false)
		maxPasswordAge = t.splitAndReadConfigLine(configSplit[2], false)
		minPasswordLength = t.splitAndReadConfigLine(configSplit[3], false)
		lengthPasswordHistoryMaintained = t.splitAndReadConfigLine(configSplit[4], false)
		lockoutThreshold = t.splitAndReadConfigLine(configSplit[5], false)
		lockoutDuration = t.splitAndReadConfigLine(configSplit[6], false)
		lockoutObservationWindow = t.splitAndReadConfigLine(configSplit[7], false)
		computerRole = t.splitAndReadConfigLine(configSplit[8], true)
		// The 9th - 12th lines do not contain useful output
	} else {
		level.Debug(t.logger).Log(
			"msg", configOutput,
			"err", "net accounts returned unexpected output",
		)
	}

	result := map[string]string{
		"force_user_logoff":                  forceUserLogoff,
		"min_password_age":                   minPasswordAge,
		"max_password_age":                   maxPasswordAge,
		"min_password_length":                minPasswordLength,
		"length_password_history_maintained": lengthPasswordHistoryMaintained,
		"lockout_threshold":                  lockoutThreshold,
		"lockout_duration":                   lockoutDuration,
		"lockout_observation_window":         lockoutObservationWindow,
		"computer_role":                      computerRole,
	}
	results = append(results, result)

	return results, nil
}

// Returns 0 if non numeric string is passed. This is useful for
// several situations in `net accounts` where you have values
// that can either be numbers or "None" / "Never" which all
// should just be zero
func (t *Table) splitAndReadConfigLine(line string, skipConversionCheck bool) string {
	val := strings.TrimSpace(strings.Split(line, ":")[1])

	if skipConversionCheck {
		return val
	}

	if _, err := strconv.Atoi(val); err == nil {
		return val
	}

	return "0"
}

func (t *Table) execNetAccounts(ctx context.Context) (string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "net", "accounts")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	level.Debug(t.logger).Log("msg", "calling net accounts")

	if err := cmd.Run(); err != nil {
		return "", errors.Wrapf(err, "calling net accounts. Got: %s", string(stderr.Bytes()))
	}

	return string(stdout.Bytes()), nil
}
