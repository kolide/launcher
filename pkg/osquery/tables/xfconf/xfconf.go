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
	"golang.org/x/exp/maps"
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

	return table.NewPlugin("kolide_xfconf", dataflattentable.Columns(table.TextColumn("username")), t.generate)
}

func (t *XfconfQuerier) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	users := tablehelpers.GetConstraints(queryContext, "username")
	if len(users) < 1 {
		return results, errors.New("kolide_xfconf requires at least one username to be specified")
	}

	// Get default config that will apply to all users unless overridden
	defaultConfig, err := t.getDefaultConfig()
	if err != nil {
		return results, fmt.Errorf("could not get default config: %w", err)
	}

	// For each user, fetch their config, check against constraints, and add to results
	for _, username := range users {
		u, err := user.Lookup(username)
		if err != nil {
			return nil, fmt.Errorf("finding user by username '%s': %w", username, err)
		}

		userRows, err := t.generateForUser(u, queryContext, defaultConfig)
		if err != nil {
			return nil, fmt.Errorf("generating rows for user '%s': %w", username, err)
		}

		results = append(results, userRows...)
	}

	return results, nil
}

// getDefaultConfig reads default xfconf settings from the filesystem
func (t *XfconfQuerier) getDefaultConfig() (map[string]interface{}, error) {
	results := make(map[string]interface{}, 0)

	defaultDirs := getDefaultXfconfDirs()
	for _, dir := range defaultDirs {
		defaultConfig, err := t.getConfigFromDirectory(dir)
		if err != nil {
			return nil, fmt.Errorf("error getting config from default directory %s: %w", dir, err)
		}
		maps.Copy(results, defaultConfig)
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

// generateForUser returns flattened rows for the given user.
func (t *XfconfQuerier) generateForUser(u *user.User, queryContext table.QueryContext, defaultConfig map[string]interface{}) ([]map[string]string, error) {
	var results []map[string]string

	// Fetch the user's config from the filesystem once, so we don't have to do it
	// repeatedly for each constraint
	userConfig, err := t.getUserConfig(u)
	if err != nil {
		return nil, fmt.Errorf("getting user config for user %s: %w", u.Username, err)
	}

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		userConfig, err := t.getCombinedFlattenedConfig(u, userConfig, defaultConfig, dataQuery)
		if err != nil {
			return nil, fmt.Errorf("could not get xfconf settings for user %s and query %s: %w", u.Username, dataQuery, err)
		}

		results = append(results, userConfig...)
	}

	return results, nil
}

// getUserConfig reads user-specific xfconf settings from the filesystem
func (t *XfconfQuerier) getUserConfig(u *user.User) (map[string]interface{}, error) {
	userConfigDir := getUserXfconfDir(u)
	userConfig, err := t.getConfigFromDirectory(userConfigDir)
	if err != nil {
		return nil, fmt.Errorf("error getting config for user %s from directory %s: %w", u.Name, userConfigDir, err)
	}

	return userConfig, nil
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

// getConfigFromDirectory expects a `dir` that contains per-channel xfconf xml files. It parses
// each XML file in the directory and returns the result as a slice of unflattened maps.
func (t *XfconfQuerier) getConfigFromDirectory(dir string) (map[string]interface{}, error) {
	results := make(map[string]interface{}, 0)

	matches, err := filepath.Glob(filepath.Join(dir, "*.xml"))
	if err != nil {
		return nil, fmt.Errorf("could not glob for files in directory %s: %w", dir, err)
	}

	for _, match := range matches {
		parsed, err := parseXfconfXml(match)
		if err != nil {
			return nil, fmt.Errorf("could not read in xml file %s: %w", match, err)
		}

		maps.Copy(results, parsed)
	}

	return results, nil
}

// getCombinedFlattenedConfig flattens and combines the given user config and default config;
// in the case of duplicate settings, it takes the value from the user config.
func (t *XfconfQuerier) getCombinedFlattenedConfig(u *user.User, userConfig map[string]interface{}, defaultConfig map[string]interface{}, dataQuery string) ([]map[string]string, error) {
	var results []map[string]string

	flattenOpts := []dataflatten.FlattenOpts{
		dataflatten.WithLogger(t.logger),
		dataflatten.WithQuery(strings.Split(dataQuery, "/")),
	}

	rowData := map[string]string{"username": u.Username}

	// Flatten user-specific settings
	userConfigRows, err := dataflatten.Flatten(userConfig, flattenOpts...)
	if err != nil {
		return results, fmt.Errorf("could not flatten user settings for user %s: %w", u.Username, err)
	}
	results = append(results, dataflattentable.ToMap(userConfigRows, dataQuery, rowData)...)

	// Add in the default settings
	defaultConfigRows, err := dataflatten.Flatten(defaultConfig, flattenOpts...)
	if err != nil {
		return results, fmt.Errorf("could not flatten default settings: %w", err)
	}
	results = append(results, dataflattentable.ToMap(defaultConfigRows, dataQuery, rowData)...)

	// Deduplicate the user and default configs, by taking the first instance in the results array
	return deduplicate(results), nil
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
