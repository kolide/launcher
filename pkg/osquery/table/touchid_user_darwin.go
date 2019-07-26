package table

import (
	"bytes"
	"context"
	"io/ioutil"
	"os/exec"
	"path/filepath"
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

	return table.NewPlugin("touchid_user_config", columns, t.generate)
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

func enumerateUIDs() ([]int, error) {
	// Enumerate all of the files in /Users
	files, err := ioutil.ReadDir("/Users")
	if err != nil {
		return nil, errors.Wrap(err, "enumerating files and folders in /Users")
	}

	// If it's a directory, add it to the array
	user_folders := make([]string, 0)
	for _, file := range files {
		if file.IsDir() {
			user_folders = append(user_folders, filepath.Join("/Users", file.Name()))
		}
	}

	// Run dscl on each directory to get its owner's UID and add it to the slice
	uids := make([]int, 0)
	for _, folder := range user_folders {
		var stdout bytes.Buffer
		cmd := exec.Command("/usr/bin/dscl", ".", "-read", folder, "UniqueID")
		cmd.Stdout = &stdout
		cmd.Run()
		outStr := string(stdout.Bytes())
		uid := strings.Split(outStr, ": ")[1]
		uid = uid[:len(uid)-1] // trim newline
		uidInt, _ := strconv.Atoi(uid)
		uids = append(uids, uidInt)
	}

	// If the length of the UIDs slice is zero, something went wrong
	if len(uids) == 0 {
		return nil, errors.Wrap(errors.New("result_length"), "UIDs slice length is zero")
	}

	return uids, nil
}

// TouchIDUserConfigGenerate will be called whenever the table is queried.
func (t *touchIDUserConfigTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	// Enumerate the uids on the system
	uids, err := enumerateUIDs()
	if err != nil {
		return nil, errors.Wrap(err, "error calling enumerateUIDs")
	}

	for _, uid := range uids {
		var touchIDUnlock, touchIDApplePay, effectiveUnlock, effectiveApplePay string
		var stdout bytes.Buffer

		// Grab the user's TouchID configuration
		cmd := exec.Command("/usr/bin/bioutil", "-r")
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
				"err", "bad_output",
			)
			continue
		}

		// Grab the fingerprint count
		stdout.Reset()
		cmd = exec.Command("/usr/bin/bioutil", "-c")
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
