//+build darwin

// Package profiles provides a table wrapper around the various
// profiles options.
//
// As the returned data is a complex nested plist, this uses the
// dataflatten tooling. (See
// https://godoc.org/github.com/kolide/launcher/pkg/dataflatten)

package profiles

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

const (
	profilesPath          = "/usr/bin/profiles"
	userAllowedCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
	typeAllowedCharacters = "abcdefghijklmnopqrstuvwxyz"
)

var (
	allowedCommands = []string{"show", "list", "status"} // Consider "sync" but that's a write comand
)

type Table struct {
	client    *osquery.ExtensionManagerClient
	logger    log.Logger
	tableName string
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {

	// profiles options. See `man profiles`. These may not be needed,
	// we use `show -all` as the default, and it probably covers
	// everything.
	columns := dataflattentable.Columns(
		table.TextColumn("user"),
		table.TextColumn("command"),
		table.TextColumn("type"),
	)

	t := &Table{
		client:    client,
		logger:    logger,
		tableName: "kolide_profiles",
	}

	return table.NewPlugin(t.tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	for _, command := range tablehelpers.GetConstraints(queryContext, "command", tablehelpers.WithAllowedValues(allowedCommands), tablehelpers.WithDefaults("show")) {
		for _, profileType := range tablehelpers.GetConstraints(queryContext, "type", tablehelpers.WithAllowedCharacters(typeAllowedCharacters), tablehelpers.WithDefaults("")) {
			for _, user := range tablehelpers.GetConstraints(queryContext, "user", tablehelpers.WithAllowedCharacters(userAllowedCharacters), tablehelpers.WithDefaults("_all")) {
				for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {

					// apple documents `-output stdout-xml` as sending the
					// output to stdout, in xml. This, however, does not work
					// for some subset of the profiles command. I've reported it
					// to apple (feedback FB8962811), and while it may someday
					// be fixed, we need to support it where it is.
					dir, err := ioutil.TempDir("", "kolide_profiles")
					if err != nil {
						return nil, errors.Wrap(err, "creating kolide_profiles tmp dir")
					}
					defer os.RemoveAll(dir)

					outputFile := filepath.Join(dir, "output.xml")

					profileArgs := []string{command, "-output", outputFile}

					if profileType != "" {
						profileArgs = append(profileArgs, "-type", profileType)
					}

					// setup the command line. This table overloads the `user`
					// column so one can select either:
					//   * All profiles merged, using the special value `_all` (this is the default)
					//   * The device profiles, using the special value `_device`
					//   * a user specific one, using the username
					switch {
					case user == "" || user == "_all":
						profileArgs = append(profileArgs, "-all")
					case user == "_device":
						break
					case user != "":
						profileArgs = append(profileArgs, "-user", user)
					default:
						return nil, errors.Errorf("Unknown user argument: %s", user)
					}

					if _, err := t.execProfiles(ctx, profileArgs); err != nil {
						level.Info(t.logger).Log("msg", "exec failed", "err", err)
						continue
					}

					flattenOpts := []dataflatten.FlattenOpts{
						dataflatten.WithQuery(strings.Split(dataQuery, "/")),
					}
					if t.logger != nil {
						flattenOpts = append(flattenOpts,
							dataflatten.WithLogger(level.NewFilter(t.logger, level.AllowInfo())),
						)
					}

					flatData, err := dataflatten.PlistFile(outputFile, flattenOpts...)
					if err != nil {
						level.Info(t.logger).Log("msg", "flatten failed", "err", err)
						continue
					}

					rowData := map[string]string{
						"command": command,
						"type":    profileType,
						"user":    user,
					}

					results = append(results, dataflattentable.ToMap(flatData, dataQuery, rowData)...)

				}
			}
		}
	}
	return results, nil
}

func (t *Table) flattenOutput(dataQuery string, systemOutput []byte) ([]dataflatten.Row, error) {
	flattenOpts := []dataflatten.FlattenOpts{}

	if dataQuery != "" {
		flattenOpts = append(flattenOpts, dataflatten.WithQuery(strings.Split(dataQuery, "/")))
	}

	if t.logger != nil {
		flattenOpts = append(flattenOpts,
			dataflatten.WithLogger(level.NewFilter(t.logger, level.AllowInfo())),
		)
	}

	return dataflatten.Plist(systemOutput, flattenOpts...)
}

func (t *Table) execProfiles(ctx context.Context, args []string) ([]byte, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, profilesPath, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	level.Debug(t.logger).Log("msg", "calling profiles", "args", cmd.Args)

	if err := cmd.Run(); err != nil {
		return nil, errors.Wrapf(err, "calling profiles. Got: %s", string(stderr.Bytes()))
	}

	// Check for an error about root permissions
	if bytes.Contains(stdout.Bytes(), []byte("requires root privileges")) {
		return nil, errors.New("Requires root privileges")
	}

	return stdout.Bytes(), nil
}
