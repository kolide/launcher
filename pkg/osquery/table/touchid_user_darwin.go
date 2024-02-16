package table

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/osquery/osquery-go/plugin/table"
)

func TouchIDUserConfig(slogger *slog.Logger) *table.Plugin {
	t := &touchIDUserConfigTable{
		slogger: slogger.With("table", "kolide_touchid_user_config"),
	}
	columns := []table.ColumnDefinition{
		table.IntegerColumn("uid"),
		table.IntegerColumn("fingerprints_registered"),
		table.IntegerColumn("touchid_unlock"),
		table.IntegerColumn("touchid_applepay"),
		table.IntegerColumn("effective_unlock"),
		table.IntegerColumn("effective_applepay"),
	}

	return table.NewPlugin("kolide_touchid_user_config", columns, t.generate)
}

type touchIDUserConfigTable struct {
	slogger *slog.Logger
}

func (t *touchIDUserConfigTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	q := queryContext.Constraints["uid"]
	if len(q.Constraints) == 0 {
		t.slogger.Log(ctx, slog.LevelDebug,
			"table requires a uid constraint, but none provided",
		)
		return nil, errors.New("The touchid_user_config table requires that you specify a constraint WHERE uid =")
	}

	var results []map[string]string
	for _, constraint := range q.Constraints {
		var touchIDUnlock, touchIDApplePay, effectiveUnlock, effectiveApplePay string

		// Verify the user exists on the system before proceeding
		_, err := user.LookupId(constraint.Expression)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelDebug,
				"nonexistent user",
				"uid", constraint.Expression,
				"err", err,
			)
			continue
		}
		uid, _ := strconv.Atoi(constraint.Expression)

		// Get the user's TouchID config
		configOutput, err := runCommandContext(ctx, uid, allowedcmd.Bioutil, "-r")
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"could not run bioutil -r",
				"uid", uid,
				"err", err,
			)
			continue
		}
		configSplit := strings.Split(configOutput, ":")

		// If the length of the split is 2, TouchID is not configured for this user
		// Otherwise, extract the values from the split.
		if len(configSplit) == 2 {
			touchIDUnlock, touchIDApplePay, effectiveUnlock, effectiveApplePay = "0", "0", "0", "0"
		} else if len(configSplit) == 6 {
			touchIDUnlock = configSplit[2][1:2]
			touchIDApplePay = configSplit[3][1:2]
			effectiveUnlock = configSplit[4][1:2]
			effectiveApplePay = configSplit[5][1:2]
		} else {
			t.slogger.Log(ctx, slog.LevelDebug,
				"bioutil -r returned unexpected output",
				"uid", uid,
				"output", configOutput,
			)
			continue
		}

		// Grab the fingerprint count
		countOutStr, err := runCommandContext(ctx, uid, allowedcmd.Bioutil, "-c")
		if err != nil {
			t.slogger.Log(ctx, slog.LevelDebug,
				"could not run bioutil -c",
				"uid", uid,
				"err", err,
			)
			continue
		}
		countSplit := strings.Split(countOutStr, ":")
		fingerprintCount := strings.ReplaceAll(countSplit[1], "\t", "")[:1]

		// If the fingerprint count is 0, set effective values to 0
		// This is due to a bug in `bioutil -r` incorrectly always returning 1
		// See https://github.com/kolide/launcher/pull/502#pullrequestreview-284351577
		if fingerprintCount == "0" {
			effectiveApplePay, effectiveUnlock = "0", "0"
		}

		result := map[string]string{
			"uid":                     strconv.Itoa(uid),
			"fingerprints_registered": fingerprintCount,
			"touchid_unlock":          touchIDUnlock,
			"touchid_applepay":        touchIDApplePay,
			"effective_unlock":        effectiveUnlock,
			"effective_applepay":      effectiveApplePay,
		}
		results = append(results, result)
	}

	return results, nil
}

// runCommand runs a given command and arguments as the supplied user
func runCommandContext(ctx context.Context, uid int, cmd allowedcmd.AllowedCommand, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Set up the command
	var stdout bytes.Buffer
	c, err := cmd(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("creating command: %w", err)
	}
	c.Stdout = &stdout

	// Check if the supplied UID is that of the current user
	currentUser, err := user.Current()
	if err != nil {
		return "", err
	}
	if strconv.Itoa(uid) != currentUser.Uid {
		c.SysProcAttr = &syscall.SysProcAttr{}
		c.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: 20}
	}

	// Run the command
	if err := c.Run(); err != nil {
		return "", err
	}

	return stdout.String(), nil
}
