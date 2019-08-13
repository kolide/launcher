package table

import (
	"bytes"
	"context"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"

	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

func TouchIDUserConfig(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	t := &touchIDUserConfigTable{
		client: client,
		logger: logger,
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
	client *osquery.ExtensionManagerClient
	logger log.Logger
	config *touchIDUserConfig
}

type touchIDUserConfig struct {
	uid                    int
	fingerprintsRegistered int
	touchIDUnlock          int
	touchIDApplePay        int
	effectiveUnlock        int
	effectiveApplePay      int
}

func (t *touchIDUserConfigTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	q, _ := queryContext.Constraints["uid"]
	if len(q.Constraints) == 0 {
		level.Debug(t.logger).Log(
			"msg", "The touchid_user_config table requires that you specify a constraint WHERE uid =",
			"err", "no constraints",
		)
		return nil, errors.New("The touchid_user_config table requires that you specify a constraint WHERE uid =")
	}

	var results []map[string]string
	for _, constraint := range q.Constraints {
		var touchIDUnlock, touchIDApplePay, effectiveUnlock, effectiveApplePay string

		// Verify the user exists on the system before proceeding
		_, err := user.LookupId(constraint.Expression)
		if err != nil {
			level.Debug(t.logger).Log(
				"msg", "nonexistant user",
				"uid", constraint.Expression,
				"err", err,
			)
			continue
		}
		uid, _ := strconv.Atoi(constraint.Expression)

		// Get the user's TouchID config
		configOutput, err := runCommandContext(ctx, uid, "/usr/bin/bioutil", "-r")
		if err != nil {
			level.Debug(t.logger).Log(
				"msg", "could not run bioutil -r",
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
			level.Debug(t.logger).Log(
				"msg", configOutput,
				"uid", uid,
				"err", "bioutil -r returned unexpected output",
			)
			continue
		}

		// Grab the fingerprint count
		countOutStr, err := runCommandContext(ctx, uid, "/usr/bin/bioutil", "-c")
		if err != nil {
			level.Debug(t.logger).Log(
				"msg", "could not run bioutil -c",
				"uid", uid,
				"err", err,
			)
			continue
		}
		countSplit := strings.Split(countOutStr, ":")
		fingerprintCount := strings.ReplaceAll(countSplit[1], "\t", "")[:1]

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
func runCommandContext(ctx context.Context, uid int, cmd string, args ...string) (string, error) {
	// Set up the command
	var stdout bytes.Buffer
	c := exec.CommandContext(ctx, cmd, args...)
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

	return string(stdout.Bytes()), nil
}
