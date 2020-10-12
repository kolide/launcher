package gsettings

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

const gsettingsPath = "/usr/bin/gsettings"

const allowedCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-."

type gsettingsExecer func(ctx context.Context, uid int, logger log.Logger, buf *bytes.Buffer) error

type GsettingsValues struct {
	client   *osquery.ExtensionManagerClient
	logger   log.Logger
	getBytes gsettingsExecer
}

// Settings returns a table plugin for querying setting values from the
// gsettings command.
func Settings(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("schema"),
		table.TextColumn("key"),
		table.TextColumn("value"),
		table.TextColumn("user"),
	}

	t := &GsettingsValues{
		client:   client,
		logger:   logger,
		getBytes: execGsettings,
	}

	return table.NewPlugin("kolide_gsettings", columns, t.generate)
}

func (t *GsettingsValues) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	users := tablehelpers.GetConstraints(queryContext, "user", tablehelpers.WithAllowedCharacters(allowedCharacters))
	if len(users) < 1 {
		return results, errors.New("kolide_gsettings requires at least one user id to be specified")
	}

	for _, u := range users {
		var output bytes.Buffer
		uid, err := strconv.Atoi(u)
		if err != nil {
			level.Info(t.logger).Log("msg", "unable to cast supplied uid as int", "uid", uid)
		}
		err = t.getBytes(ctx, uid, t.logger, &output)
		if err != nil {
			// assume that getBytes logs the error.. might be better to log it here, but for debugging it's nice to log it in getBytes to append the stdout and stderr
			// return results, errors.Wrap(err, "calling getbytes")
			return results, errors.Wrap(err, "getting bytes")
		}

		user_results := t.parse(&output)
		for _, r := range user_results {
			r["user"] = u
			results = append(results, r)
		}
	}

	return results, nil
}

// execGsettings writes the output of running 'gsettings' command into the supplied bytes buffer
// TODO: would maybe be cleaner to make logger a functional option..
func execGsettings(ctx context.Context, uid int, l log.Logger, buf *bytes.Buffer) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	u, err := user.LookupId(string(uid))
	if err != nil {
		return errors.Wrap(err, "looking up user by uid")
	}

	// cmd := exec.CommandContext(ctx, gsettingsPath, "list-recursively", "--schemadir", u.HomeDir)
	cmd := exec.CommandContext(ctx, gsettingsPath, "list-recursively")

	// EXPERIMENTAL: uncommenting this seems to cause no results to be returned.
	// set the HOME for the the cmd so that gsettings is exec'd properly as the
	// new user.
	cmd.Env = append(os.Environ(), fmt.Sprintf("HOME=%s", u.HomeDir))

	// Check if the supplied UID is that of the current user
	currentUser, err := user.Current()
	if err != nil {
		return errors.Wrap(err, "checking current user uid")
	}

	// adding a cmd.SysProcAttr.Credential sets the user the command is run
	// under, as identified by the uid
	if strconv.Itoa(uid) != currentUser.Uid {
		// guid, err := strconv.Atoi(u.Gid)
		// if err != nil {
		// 	return errors.Wrap(err, "converting guid from string to int")
		// }
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{
			Uid: uint32(uid),
			Gid: 20,
		}
	}

	dir, err := ioutil.TempDir("", "osq-gsettings")
	if err != nil {
		return errors.Wrap(err, "mktemp")
	}
	defer os.RemoveAll(dir)

	if err := os.Chmod(dir, 0755); err != nil {
		return errors.Wrap(err, "chmod")
	}
	cmd.Dir = dir

	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	cmd.Stdout = buf

	if err := cmd.Run(); err != nil {
		// level.Error(l).Log(
		// 	"msg", "error running gsettings",
		// 	"stderr", strings.TrimSpace(stderr.String()),
		// 	"stdout", strings.TrimSpace(buf.String()),
		// 	"cmd", cmd.Args,
		// 	"err", err,
		// )
		return errors.Wrapf(err, "exec-ing gsettings, cmd was %a, stderr: %s", cmd.Args)
	}

	return nil
}

func (t *GsettingsValues) parse(input *bytes.Buffer) []map[string]string {
	var results []map[string]string

	scanner := bufio.NewScanner(input)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// parts := strings.SplitN(line, " ", 3)
		// if len(parts) < 3 {
		// 	level.Error(t.logger).Log(
		// 		"msg", "unable to process line, not enough segments",
		// 		"line", line,
		// 	)
		// 	continue
		// }
		row := make(map[string]string)
		// row["schema"] = parts[0]
		// row["key"] = parts[1]
		// row["value"] = parts[2]

		row["value"] = line
		results = append(results, row)
	}

	if err := scanner.Err(); err != nil {
		level.Debug(t.logger).Log("msg", "scanner error", "err", err)
		panic(err)
	}

	return results
}
