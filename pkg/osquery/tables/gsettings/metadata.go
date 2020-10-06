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
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

type GsettingsMetadata struct {
	client    *osquery.ExtensionManagerClient
	logger    log.Logger
	cmdRunner func(ctx context.Context, args []string, output *bytes.Buffer) error
}

// Metadata returns a table plugin for querying metadata about specific keys in
// specific schemas
func Metadata(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {

	columns := []table.ColumnDefinition{
		// TODO: maybe need to add 'path' for relocatable schemas..
		table.TextColumn("schema"),
		table.TextColumn("key"),
		table.TextColumn("description"),
		table.TextColumn("type"),
	}

	t := &GsettingsMetadata{
		client:    client,
		logger:    logger,
		cmdRunner: execGsettingsCommand,
	}

	return table.NewPlugin("kolide_gsettings_metadata", columns, t.generate)
}

func (t *GsettingsMetadata) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {

	var results []map[string]string
	schemas := tablehelpers.GetConstraints(queryContext, "schema", tablehelpers.WithAllowedCharacters(allowedCharacters))
	if len(schemas) < 1 {
		return results, errors.New("kolide_gsettings_metadata table requires at least one schemas to be specified")
	}

	for _, schema := range schemas {
		descriptions, err := t.gsettingsDescribeForSchema(ctx, schema)
		if err != nil {
			level.Info(t.logger).Log(
				"msg", "error describing keys for schema",
				"schema", schema,
				"err", err,
			)
			continue
		}
		for _, d := range descriptions {
			row := map[string]string{
				"description": d.Description,
				"type":        d.Type,
				"schema":      schema,
				"key":         d.Key,
			}
			results = append(results, row)
		}

	}

	return results, nil
}

type datatype struct {
	raw string
}

type keyDescription struct {
	Description string
	Type        string
	Key         string
}

func (k *keyDescription) toMap() map[string]string {
	return map[string]string{
		"description": k.Description,
		"type":        k.Type,
		"key":         k.Key,
	}
}

func (t *GsettingsMetadata) gsettingsDescribeForSchema(ctx context.Context, schema string) ([]keyDescription, error) {
	var descriptions []keyDescription

	keys, err := t.listKeys(ctx, schema)
	if err != nil {
		return descriptions, errors.Wrap(err, "fetching keys to describe")
	}

	for _, k := range keys {
		desc, err := t.describeKey(ctx, schema, k)
		if err != nil {
			level.Info(t.logger).Log(
				"msg", "error describing key",
				"key", k,
				"schema", schema,
				"err", err,
			)
			continue
		}
		descriptions = append(descriptions, desc)
	}

	return descriptions, nil
}

func (t *GsettingsMetadata) listKeys(ctx context.Context, schema string) ([]string, error) {
	var keys []string
	output := new(bytes.Buffer)

	err := t.cmdRunner(ctx, []string{"list-keys", schema}, output)
	if err != nil {
		return keys, errors.Wrap(err, "fetching keys")
	}
	scanner := bufio.NewScanner(output)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		keys = append(keys, line)
	}

	if err := scanner.Err(); err != nil {
		level.Info(t.logger).Log("msg", "scanner error", "err", err)
	}

	return keys, nil
}

func (t *GsettingsMetadata) describeKey(ctx context.Context, schema, key string) (keyDescription, error) {
	desc := keyDescription{Key: key}

	d, err := t.getDescription(ctx, schema, key)
	if err != nil {
		return desc, errors.Wrap(err, "getting key's description")
	}
	desc.Description = d

	datatype, err := t.getType(ctx, key, schema)
	if err != nil {
		return desc, errors.Wrap(err, "discerning key's type")
	}
	desc.Type = datatype

	return desc, nil
}

func (t *GsettingsMetadata) getDescription(ctx context.Context, schema, key string) (string, error) {
	output := new(bytes.Buffer)

	err := t.cmdRunner(ctx, []string{"describe", schema, key}, output)
	if err != nil {
		return "", errors.Wrap(err, "describing key")
	}

	return strings.TrimSpace(output.String()), nil
}

func (t *GsettingsMetadata) getType(ctx context.Context, key, schema string) (string, error) {
	output := new(bytes.Buffer)

	err := t.cmdRunner(ctx, []string{"range", schema, key}, output)
	if err != nil {
		return "", errors.Wrap(err, "running 'gsettings range'")
	}

	result := strings.TrimSpace(strings.ReplaceAll(output.String(), "\n", " "))
	// enum types need special formatting to distinguish the type (enum) from
	// the possible values
	if strings.HasPrefix(result, "enum") {
		s := strings.TrimPrefix(result, "enum ")
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

func execGsettingsCommand(ctx context.Context, args []string, output *bytes.Buffer) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	command := args[0]
	cmd := exec.CommandContext(ctx, gsettingsPath, args...)
	dir, err := ioutil.TempDir("", fmt.Sprintf("osq-gsettings-%s", command))
	if err != nil {
		return errors.Wrap(err, "mktemp")
	}
	defer os.RemoveAll(dir)

	if err := os.Chmod(dir, 0755); err != nil {
		return errors.Wrap(err, "chmod")
	}
	cmd.Dir = dir
	cmd.Stderr = new(bytes.Buffer)
	cmd.Stdout = output

	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "running gsettings %s", command)
	}

	return nil
}

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
