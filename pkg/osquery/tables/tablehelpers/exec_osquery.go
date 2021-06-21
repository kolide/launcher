package tablehelpers

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
)

// ExecOsquery runs osquery under launchctl, in a user context.
func ExecOsquery(ctx context.Context, logger log.Logger, timeoutSeconds int, username string, osqueryPath string, query string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	targetUser, err := user.Lookup(username)
	if err != nil {
		return nil, errors.Wrapf(err, "looking up username %s", username)
	}

	cmd := exec.CommandContext(ctx,
		"launchctl",
		"asuser",
		targetUser.Uid,
		osqueryPath,
		"--config_path", "/dev/null",
		"--disable_events",
		"--disable_database",
		"--disable_audit",
		"--ephemeral",
		"-S",
		"--json",
		query,
	)

	dir, err := ioutil.TempDir("", "osq-runas")
	if err != nil {
		return nil, errors.Wrap(err, "mktemp")
	}
	defer os.RemoveAll(dir)

	if err := os.Chmod(dir, 0755); err != nil {
		return nil, errors.Wrap(err, "chmod")
	}

	cmd.Dir = dir

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout, cmd.Stderr = stdout, stderr

	if err := cmd.Run(); err != nil {
		return nil, errors.Wrapf(err, "running osquery. Got: '%s'", string(stderr.Bytes()))
	}

	return stdout.Bytes(), nil

}

func ExecOsqueryParsed(ctx context.Context, logger log.Logger, timeoutSeconds int, username string, osqueryPath string, query string) ([]map[string]string, error) {
	outBytes, err := ExecOsquery(ctx, logger, timeoutSeconds, username, osqueryPath, query)
	if err != nil {
		return nil, err
	}

	var osqueryResults []map[string]string

	if err := json.Unmarshal(outBytes, &osqueryResults); err != nil {
		level.Info(logger).Log(
			"msg", "error unmarshalling json",
			"err", err,
			"stdout", string(outBytes),
		)
		return nil, errors.Wrap(err, "unmarshalling json")
	}

	return osqueryResults, nil
}
