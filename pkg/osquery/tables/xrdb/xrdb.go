// +build linux

package xrdb

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

var potentialPaths = []string{"/usr/bin/xrdb"}

const allowedCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-."

type execer func(ctx context.Context, username string, buf *bytes.Buffer) error

type XRDBSettings struct {
	client   *osquery.ExtensionManagerClient
	logger   log.Logger
	getBytes execer
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("key"),
		table.TextColumn("value"),
		table.TextColumn("username"),
	}

	t := &XRDBSettings{
		client:   client,
		logger:   logger,
		getBytes: execXRDB,
	}

	return table.NewPlugin("kolide_xrdb", columns, t.generate)
}

func (t *XRDBSettings) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	users := tablehelpers.GetConstraints(queryContext, "username", tablehelpers.WithAllowedCharacters(allowedCharacters))
	if len(users) < 1 {
		return results, errors.New("kolide_gsettings requires at least one username to be specified")
	}
	for _, username := range users {
		var output bytes.Buffer

		err := t.getBytes(ctx, username, &output)
		if err != nil {
			level.Info(t.logger).Log(
				"msg", "error getting bytes for user",
				"username", username,
				"err", err,
			)
			continue
		}

		user_results := t.parse(username, &output)
		results = append(results, user_results...)
	}

	return results, nil
}

// execXRDB writes the output of running 'gsettings' command into the
// supplied bytes buffer
func execXRDB(ctx context.Context, username string, buf *bytes.Buffer) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	u, err := user.Lookup(username)
	if err != nil {
		return errors.Wrapf(err, "finding user by username '%s'", username)
	}

	// TODO: maybe use the helpers to try multiple potential paths
	cmd := exec.CommandContext(ctx, potentialPaths[0], "-display", ":0", "-global", "-query")

	// set the HOME cmd so that xrdb is exec'd properly as the new user.
	cmd.Env = append(cmd.Env, fmt.Sprintf("HOME=%s", u.HomeDir))

	// Check if the supplied UID is that of the current user
	currentUser, err := user.Current()
	if err != nil {
		return errors.Wrap(err, "checking current user uid")
	}

	if u.Uid != currentUser.Uid {
		uid, err := strconv.ParseInt(u.Uid, 10, 32)
		if err != nil {
			return errors.Wrap(err, "converting uid from string to int")
		}
		gid, err := strconv.ParseInt(u.Gid, 10, 32)
		if err != nil {
			return errors.Wrap(err, "converting gid from string to int")
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		}
	}

	dir, err := ioutil.TempDir("", "osq-xrdb")
	if err != nil {
		return errors.Wrap(err, "mktemp")
	}
	defer os.RemoveAll(dir)

	// if we don't chmod the dir, we get errors like:
	// fork/exec /usr/bin/gsettings: permission denied'
	if err := os.Chmod(dir, 0755); err != nil {
		return errors.Wrap(err, "chmod")
	}

	cmd.Dir = dir

	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	cmd.Stdout = buf

	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "running xrdb, err is: %s", stderr.String())
	}

	return nil
}

func (t *XRDBSettings) parse(username string, input io.Reader) []map[string]string {
	var results []map[string]string

	scanner := bufio.NewScanner(input)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			level.Error(t.logger).Log(
				"msg", "unable to process line, not enough segments",
				"line", line,
			)
			continue
		}
		row := make(map[string]string)
		row["key"] = parts[0]
		row["value"] = strings.TrimSpace(parts[1])
		row["username"] = username

		results = append(results, row)
	}

	if err := scanner.Err(); err != nil {
		level.Debug(t.logger).Log("msg", "scanner error", "err", err)
	}

	return results
}
