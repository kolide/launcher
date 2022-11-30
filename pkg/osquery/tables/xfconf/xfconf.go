//go:build !windows
// +build !windows

package xfconf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
)

// Provides configuration settings for devices using XFCE desktop environment.
// See https://docs.xfce.org/xfce/xfce4-session/advanced#files_and_environment_variables
// for a list of the files and environment variables we check for configuration.

var xfconfChannelXmlPath string = filepath.Join("xfce4", "xfconf", "xfce-perchannel-xml")

type XfconfQuerier struct {
	logger log.Logger
}

func TablePlugin(logger log.Logger) *table.Plugin {
	t := &XfconfQuerier{
		logger: logger,
	}

	return table.NewPlugin("kolide_xfconf", dataflattentable.Columns(table.TextColumn("username"), table.TextColumn("channel")), t.generate)
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

		rowData := map[string]string{"username": username}

		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			userConfig, err := t.getUserConfig(u, dataQuery, rowData)
			if err != nil {
				return nil, fmt.Errorf("could not get xfconf settings for user %s and query %s: %w", username, dataQuery, err)
			}

			results = append(results, userConfig...)
		}
	}

	return results, nil
}

func (t *XfconfQuerier) getUserConfig(u *user.User, dataQuery string, rowData map[string]string) ([]map[string]string, error) {
	var results []map[string]string

	// First, get user-specific settings
	userConfigDir := getUserXfconfDir(u)
	userConfigRows, err := t.getConfigFromDirectory(userConfigDir, dataQuery, rowData)
	if err != nil {
		return nil, fmt.Errorf("error getting config for user %s from directory %s: %w", u.Name, userConfigDir, err)
	}
	results = append(results, userConfigRows...)

	// Then, get defaults
	defaultDirs := getDefaultXfconfDirs()
	for _, dir := range defaultDirs {
		defaultConfigRows, err := t.getConfigFromDirectory(dir, dataQuery, rowData)
		if err != nil {
			return nil, fmt.Errorf("error getting config for user %s from default directory %s: %w", u.Name, dir, err)
		}
		results = append(results, defaultConfigRows...)
	}

	// Make sure we only include defaults if there isn't already a user-specific setting
	return deduplicate(results), nil
}

func (t *XfconfQuerier) getConfigFromDirectory(dir string, dataQuery string, rowData map[string]string) ([]map[string]string, error) {
	var results []map[string]string

	matches, err := filepath.Glob(filepath.Join(dir, "*.xml"))
	if err != nil {
		return nil, fmt.Errorf("could not glob for files in directory %s: %w", dir, err)
	}

	flattenOpts := []dataflatten.FlattenOpts{
		dataflatten.WithLogger(t.logger),
		dataflatten.WithQuery(strings.Split(dataQuery, "/")),
	}

	for _, match := range matches {
		flattened, err := parseXfconfXml(match, flattenOpts...)
		if err != nil {
			return nil, fmt.Errorf("could not read in xml file %s: %w", match, err)
		}
		rowData["channel"] = strings.TrimSuffix(filepath.Base(match), ".xml")

		results = append(results, dataflattentable.ToMap(flattened, dataQuery, rowData)...)
	}

	return results, nil
}

// getDefaultXfconfDirs returns the path to xfconf per-channel xml files that contain
// default settings. It checks for an override, otherwise defaults to /etc/xdg/...
// See https://docs.xfce.org/xfce/xfce4-session/advanced#files_and_environment_variables.
func getDefaultXfconfDirs() []string {
	envDefaultDirsStr := os.Getenv("XDG_CONFIG_DIRS")
	if envDefaultDirsStr != "" {
		dirs := strings.Split(envDefaultDirsStr, ":")
		for i, d := range dirs {
			dirs[i] = filepath.Join(d, xfconfChannelXmlPath)
		}
		return dirs
	}

	return []string{filepath.Join("/", "etc", "xdg", xfconfChannelXmlPath)}
}

// getUserXfconfDir returns to xfconf per-channel xml files that contain user-specific
// settings. It checks for an override via environment variable, otherwise defaults to
// ~/.config/...
// See https://docs.xfce.org/xfce/xfce4-session/advanced#files_and_environment_variables.
func getUserXfconfDir(u *user.User) string {
	userConfigDir := os.Getenv("XDG_CONFIG_HOME")
	if userConfigDir != "" {
		return filepath.Join(userConfigDir, xfconfChannelXmlPath)
	}

	return filepath.Join(u.HomeDir, ".config", xfconfChannelXmlPath)
}

// deduplicate takes an array of rows that may have duplicate keys and deduplicates,
// always taking the first instance of the row and discarding subsequent ones.
func deduplicate(rows []map[string]string) []map[string]string {
	var deduplicated []map[string]string

	seenKeys := make(map[string]bool)

	for _, row := range rows {
		if _, ok := seenKeys[row["fullkey"]]; !ok {
			seenKeys[row["fullkey"]] = true
			deduplicated = append(deduplicated, row)
		}
	}

	return deduplicated
}
