//go:build !windows
// +build !windows

package xfconf

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
)

// Provides configuration settings for devices using XFCE desktop environment.
// See https://docs.xfce.org/xfce/xfconf/xfconf-query for documentation on the
// tool we use to query this data.

const xfconfQueryPath = "/usr/bin/xfconf-query"

type XfconfQuerier struct {
	logger log.Logger
}

func TablePlugin(logger log.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("channel"),
		table.TextColumn("key"),
		table.TextColumn("value"),
		table.TextColumn("username"),
	}

	t := &XfconfQuerier{
		logger: logger,
	}

	return table.NewPlugin("kolide_xfconf", columns, t.generate)
}

func (t *XfconfQuerier) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	users := tablehelpers.GetConstraints(queryContext, "username")
	if len(users) < 1 {
		return results, errors.New("kolide_xfconf requires at least one username to be specified")
	}

	for _, username := range users {
		u, err := user.Lookup(username)
		if err != nil {
			return nil, fmt.Errorf("finding user by username '%s': %w", username, err)
		}

		channels, err := t.getChannels(ctx, u, queryContext)
		if err != nil {
			return nil, fmt.Errorf("finding channels for user %s: %w", username, err)
		}

		for _, channel := range channels {
			properties, err := t.getProperties(ctx, channel, u, queryContext)
			if err != nil {
				return nil, fmt.Errorf("finding properties on channel %s for user %s: %w", channel, username, err)
			}

			results = append(results, properties...)
		}
	}

	return results, nil
}

func (t *XfconfQuerier) getChannels(ctx context.Context, u *user.User, queryContext table.QueryContext) ([]string, error) {
	listChannelsCmd := exec.CommandContext(ctx, xfconfQueryPath, "--list")
	var output bytes.Buffer
	if err := setUserForCommandAndRun(listChannelsCmd, u, &output); err != nil {
		return nil, fmt.Errorf("listing channels for user %s: %w", u.Name, err)
	}

	// Output looks like:
	// Channels:
	// 		xfce4-session
	//		xsettings
	//
	// Skip first line, and strip whitespace
	scanner := bufio.NewScanner(&output)
	channels := make([]string, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line == "Channels:" {
			continue
		}

		channels = append(channels, line)
	}

	return tablehelpers.GetConstraints(queryContext, "channel", tablehelpers.WithAllowedValues(channels)), nil
}

func (t *XfconfQuerier) getProperties(ctx context.Context, channel string, u *user.User, queryContext table.QueryContext) ([]map[string]string, error) {
	cmdArgs := []string{
		"--channel", channel,
	}

	// If the query specified a property, then query for that only
	propertyConstraints := tablehelpers.GetConstraints(queryContext, "key")
	if len(propertyConstraints) == 1 {
		cmdArgs = append(cmdArgs, "--property", propertyConstraints[0])
	}

	// Including --verbose will get us properties paired with their values
	cmdArgs = append(cmdArgs, "--list", "--verbose")

	listPropertiesCmd := exec.CommandContext(ctx, xfconfQueryPath, cmdArgs...)
	var output bytes.Buffer
	if err := setUserForCommandAndRun(listPropertiesCmd, u, &output); err != nil {
		return nil, fmt.Errorf("listing properties on channel %s for user %s: %w", channel, u.Name, err)
	}

	return t.parseProperties(&output, channel, u), nil
}

func (t *XfconfQuerier) parseProperties(input io.Reader, channel string, u *user.User) []map[string]string {
	var results []map[string]string

	scanner := bufio.NewScanner(input)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Each line is in the following format:
		// /some/path/to/setting		setting-value
		// Split the line on the whitespace to obtain the key and value.
		parts := strings.Fields(line)
		if len(parts) > 2 || len(parts) == 0 {
			level.Error(t.logger).Log(
				"msg", "unable to process line -- invalid number of values",
				"line", line,
			)
			continue
		}
		fullKey := parts[0]

		var val string
		if len(parts) == 2 {
			val = parts[1]
		}

		row := make(map[string]string)
		row["channel"] = channel
		row["key"] = fullKey
		row["value"] = val
		row["username"] = u.Name

		results = append(results, row)
	}

	if err := scanner.Err(); err != nil {
		level.Debug(t.logger).Log("msg", "scanner error", "err", err)
	}

	return results
}

func setUserForCommandAndRun(cmd *exec.Cmd, u *user.User, output *bytes.Buffer) error {
	// Set the HOME for the cmd so that the command is executed as the intended user
	cmd.Env = append(cmd.Env, fmt.Sprintf("HOME=%s", u.HomeDir))

	// Check if the supplied UID is that of the current user
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("checking current user uid: %w", err)
	}

	// Nothing else to do here
	if u.Uid == currentUser.Uid {
		return nil
	}

	// Set UID, GID on command
	uid, err := strconv.ParseInt(u.Uid, 10, 32)
	if err != nil {
		return fmt.Errorf("converting uid from string to int: %w", err)
	}
	gid, err := strconv.ParseInt(u.Gid, 10, 32)
	if err != nil {
		return fmt.Errorf("converting gid from string to int: %w", err)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Credential = &syscall.Credential{
		Uid: uint32(uid),
		Gid: uint32(gid),
	}

	// Set output
	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	cmd.Stdout = output

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error running xfconf-query for user %s with args %v and env %v; err is: %s: %w", u.Name, cmd.Args, cmd.Env, stderr.String(), err)
	}

	return nil
}
