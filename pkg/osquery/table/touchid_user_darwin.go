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
		var stdout bytes.Buffer

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

		// Grab the user's TouchID configuration
		cmd := exec.CommandContext(ctx, "/usr/bin/bioutil", "-r")
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: 20}
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			level.Debug(t.logger).Log(
				"msg", "Failed to run bioutil for configuration",
				"uid", uid,
				"err", err,
			)
			continue
		}
		configOutStr := string(stdout.Bytes())
		configSplit := strings.Split(configOutStr, ":")

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
				"msg", "bioutil -r returned unexpected output",
				"uid", uid,
				"err", "bad output",
			)
			continue
		}

		// Grab the fingerprint count
		stdout.Reset()
		cmd = exec.CommandContext(ctx, "/usr/bin/bioutil", "-c")
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: 20}
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			level.Debug(t.logger).Log(
				"msg", "Failed to run bioutil for fingerprint count",
				"uid", uid,
				"err", err,
			)
			continue
		}
		countOutStr := string(stdout.Bytes())
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
