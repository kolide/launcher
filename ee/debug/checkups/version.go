package checkups

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/Masterminds/semver"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/ee/agent/types"
)

type Version struct {
	k       types.Knapsack
	status  Status
	summary string
}

func (c *Version) Name() string {
	return "Launcher Version"
}

func (c *Version) Run(_ context.Context, fullFH io.Writer) error {
	version := version.Version().Version

	if _, err := semver.NewVersion(version); err != nil {
		c.status = Failing
		c.summary = fmt.Sprintf("(%s) %s", version, err)
		return nil
	}

	c.status = Passing
	c.summary = fmt.Sprintf("launcher_version %s", version)
	return nil
}

func (c *Version) ExtraFileName() string {
	return ""
}

func (c *Version) Status() Status {
	return c.status
}

func (c *Version) Summary() string {
	return c.summary
}

func (c *Version) Data() any {
	firstRunTime := ""
	firstRunVersionStr := ""

	if c.k.LauncherHistoryStore() != nil {

		runtime, err := getTimeFromStore(c.k.LauncherHistoryStore(), "first_recorded_run_time")
		if err != nil {
			c.k.Slogger().Log(context.Background(), slog.LevelDebug,
				"getting first recorded run time from store",
				"err", err,
			)
		} else {
			firstRunTime = runtime
		}

		firstRunVersionBytes, err := c.k.LauncherHistoryStore().Get([]byte("first_recorded_version"))
		if err != nil || firstRunVersionBytes == nil {
			c.k.Slogger().Log(context.Background(), slog.LevelDebug,
				"getting first recorded version from store",
				"err", err,
				"nil_value", firstRunVersionBytes == nil,
			)
		} else {
			firstRunVersionStr = string(firstRunVersionBytes)
		}
	}

	return map[string]any{
		"update_channel":          c.k.UpdateChannel(),
		"tufServer":               c.k.TufServerURL(),
		"launcher_version":        version.Version().Version,
		"first_recorded_run_time": firstRunTime,
		"first_recorded_version":  firstRunVersionStr,
	}
}

func getTimeFromStore(store types.Getter, key string) (string, error) {
	if store == nil {
		return "", errors.New("store is nil")
	}

	bytes, err := store.Get([]byte(key))
	if err != nil {
		return "", err
	}

	if bytes == nil {
		return "", nil
	}

	t, err := time.Parse(time.RFC3339, string(bytes))
	if err != nil {
		return "", err
	}

	return t.Format(time.RFC3339), nil
}
