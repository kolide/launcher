package gsettings

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

const gsettingsPath = "/usr/bin/gsettings"
const allowedCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-."

type GsettingsValues struct {
	client   *osquery.ExtensionManagerClient
	logger   log.Logger
	getBytes func(ctx context.Context, buf *bytes.Buffer) error
}

type GsettingsMetadata struct {
	client *osquery.ExtensionManagerClient
	logger log.Logger
}

// Settings returns a table plugin for querying setting values from the
// gsettings command.
func Settings(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	var columns []table.ColumnDefinition

	// we don't need the 'query' column.
	// we could also just type out all the cols we do want
	for _, col := range dataflattentable.Columns(table.TextColumn("schema")) {
		if col.Name != "query" {
			columns = append(columns, col)
		}
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
	var output bytes.Buffer

	err := t.getBytes(ctx, &output)
	if err != nil {
		level.Info(t.logger).Log("msg", "gsettings failed", "err", err)

		return results, err
	}
	data, err := t.flatten(&output)
	if err != nil {
		level.Info(t.logger).Log(
			"msg", "error flattening data",
			"err", err,
		)
		return results, err
	}

	for _, row := range data {
		p, k := row.ParentKey("/")

		res := map[string]string{
			"fullkey": row.StringPath("/"),
			"parent":  p,
			"key":     k,
			"value":   row.Value,
			"schema":  p,
		}
		results = append(results, res)
	}

	return results, nil
}

// execGsettings writes the output of running 'gsettings' command into the supplied bytes buffer
func execGsettings(ctx context.Context, buf *bytes.Buffer) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/usr/bin/gsettings", "list-recursively")
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
		return errors.Wrap(err, "running osquery")
	}

	return nil
}

func (t *GsettingsValues) flatten(buffer *bytes.Buffer) ([]dataflatten.Row, error) {
	flattenOpts := []dataflatten.FlattenOpts{}
	if t.logger != nil {
		flattenOpts = append(flattenOpts,
			dataflatten.WithLogger(level.NewFilter(t.logger, level.AllowInfo())),
		)
	}
	results := t.parse(buffer)
	var rows []dataflatten.Row

	// rows, err := dataflatten.Flatten(results)

	for _, result := range results {
		row := dataflatten.NewRow([]string{result["schema"], result["key"]}, result["value"])
		rows = append(rows, row)
	}
	return rows, nil
}

// Metadata returns a table plugin for querying metadata about specific keys in
// specific schemas
func Metadata(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {

	columns := []table.ColumnDefinition{
		// TODO: maybe need to add 'path' for relocatable schemas..
		table.TextColumn("schema"),
		table.TextColumn("key"),
		table.TextColumn("description"),
	}

	t := &GsettingsMetadata{
		client: client,
		logger: logger,
	}

	return table.NewPlugin("kolide_gsettings_metadata", columns, t.generate)
}

func (t *GsettingsMetadata) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	// TODO: Implement me
	return []map[string]string{}, nil
}

type datatype struct {
	raw string
}

type keyDescription struct {
	Description string
	Type        string
}

func (t *GsettingsValues) gsettingsDescribe(ctx context.Context, key, schema string) (keyDescription, error) {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	var desc keyDescription
	cmd := exec.CommandContext(ctx, "/usr/bin/gsettings", "describe", schema, key)
	dir, err := ioutil.TempDir("", "osq-gsettings-desc")
	if err != nil {
		return keyDescription{}, errors.Wrap(err, "mktemp")
	}
	defer os.RemoveAll(dir)

	if err := os.Chmod(dir, 0755); err != nil {
		return keyDescription{}, errors.Wrap(err, "chmod")
	}
	cmd.Dir = dir
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout, cmd.Stderr = stdout, stderr

	if err := cmd.Run(); err != nil {
		level.Debug(t.logger).Log(
			"msg", "Error running gsettings describe",
			"key", key,
			"schema", schema,
			"stderr", strings.TrimSpace(stderr.String()),
			"stdout", strings.TrimSpace(stdout.String()),
			"err", err,
		)
		return keyDescription{}, errors.Wrap(err, "running gsettings describe")
	}
	desc.Description = strings.TrimSpace(stdout.String())
	datatype, err := t.getType(ctx, key, schema)
	if err != nil {
		return desc, errors.Wrap(err, "discerning key's type")
	}
	desc.Type = datatype

	return desc, nil
}

func (t *GsettingsValues) getType(ctx context.Context, key, schema string) (string, error) {
	cmd := exec.CommandContext(ctx, "/usr/bin/gsettings", "range", schema, key)
	dir, err := ioutil.TempDir("", "osq-gsettings-range")
	if err != nil {
		return "", errors.Wrap(err, "mktemp")
	}
	defer os.RemoveAll(dir)

	if err := os.Chmod(dir, 0755); err != nil {
		return "", errors.Wrap(err, "chmod")
	}
	cmd.Dir = dir
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout, cmd.Stderr = stdout, stderr

	if err := cmd.Run(); err != nil {
		level.Debug(t.logger).Log(
			"msg", "Error running gsettings range",
			"key", key,
			"schema", schema,
			"stderr", strings.TrimSpace(stderr.String()),
			"stdout", strings.TrimSpace(stdout.String()),
			"err", err,
		)
		return "", errors.Wrap(err, "running gsettings range")
	}

	result := strings.ReplaceAll(strings.TrimSpace(stdout.String()), "\n", " ")

	// enum types need special formatting to distinguish the type (enum) from
	// the possible values
	if strings.HasPrefix(result, "enum") {
		s := strings.TrimPrefix(result, "enum")
		vals := strings.Split(s, " ")
		return fmt.Sprintf("enum: [ %s ]", strings.Join(vals, ",")), nil
	}

	// 'range' datatypes also need special handling
	if strings.HasPrefix(result, "range ") {
		s := strings.TrimPrefix(result, "range ")
		parts := strings.Split(s, " ")

		typ := convertType(parts[0])
		var scope string
		if len(parts) > 2 {
			scope = fmt.Sprintf(" (%v to %v)", parts[1], parts[2])
		}

		return fmt.Sprintf("%s%s", typ, scope), nil
	}

	return convertType(result), nil
}

// this feels.. not idiomatic
var gvariantMapping = map[string]string{
	"b": "bool",
	"n": "int16",
	"q": "uint16",
	"i": "int32",
	"u": "uint32",
	"x": "int64",
	"t": "uint64",
	"d": "double",
	"s": "string",
	"a": "array",
}

// convertType returns a string describing the GVariantType corresponding to the
// GVariant-formatted type string. see  https://developer.gnome.org/glib/unstable/glib-GVariantType.html
// for documentation. Note that not all types listed in the documentation above
// are supported, for example:
//  - tuples (e.g. tuple of 2 strings `(ss)`)
//  - nested types (e.g.// array of tuples: `a(ss)`)
// and other complex types are not supported.
func convertType(typ string) string {
	typ = strings.Replace(typ, "type ", "", 1) // remove any leading 'type ', eg 'type b'
	var prefix string
	if strings.HasPrefix(typ, "a") {
		typ = typ[1:]
		prefix = "array of "
	}
	primitive_typ, ok := gvariantMapping[typ]
	if !ok {
		return "other"
	}
	return fmt.Sprintf("%s%s", prefix, primitive_typ)
}

func (t *GsettingsValues) parse(input *bytes.Buffer) []map[string]string {
	var results []map[string]string

	scanner := bufio.NewScanner(input)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		row := make(map[string]string)
		parts := strings.SplitN(line, " ", 3)
		row["schema"] = parts[0]
		row["key"] = parts[1]
		row["value"] = parts[2]
		results = append(results, row)
	}

	if err := scanner.Err(); err != nil {
		level.Debug(t.logger).Log("msg", "scanner error", "err", err)
	}

	return results
}
