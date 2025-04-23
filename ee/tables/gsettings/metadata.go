//go:build linux
// +build linux

package gsettings

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/osquery/osquery-go/plugin/table"
)

type GsettingsMetadata struct {
	slogger   *slog.Logger
	cmdRunner func(ctx context.Context, args []string, tmpdir string, output *bytes.Buffer) error
}

// Metadata returns a table plugin for querying metadata about specific keys in
// specific schemas
func Metadata(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		// TODO: maybe need to add 'path' for relocatable schemas..
		table.TextColumn("schema"),
		table.TextColumn("key"),
		table.TextColumn("description"),
		table.TextColumn("type"),
	}

	t := &GsettingsMetadata{
		slogger:   slogger.With("table", "kolide_gsettings_metadata"),
		cmdRunner: execGsettingsCommand,
	}

	return tablewrapper.New(flags, slogger, "kolide_gsettings_metadata", columns, t.generate)
}

func (t *GsettingsMetadata) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", "kolide_gsettings_metadata")
	defer span.End()

	var results []map[string]string

	schemas := tablehelpers.GetConstraints(queryContext, "schema", tablehelpers.WithAllowedCharacters(allowedCharacters))
	if len(schemas) < 1 {
		return results, errors.New("kolide_gsettings_metadata table requires at least one schema to be specified")
	}

	for _, schema := range schemas {
		descriptions, err := t.gsettingsDescribeForSchema(ctx, schema)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"error describing keys for schema",
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

type keyDescription struct {
	Description string
	Type        string
	Key         string
}

func (t *GsettingsMetadata) gsettingsDescribeForSchema(ctx context.Context, schema string) ([]keyDescription, error) {
	ctx, span := observability.StartSpan(ctx, "schema", schema)
	defer span.End()

	var descriptions []keyDescription

	dir, err := agent.MkdirTemp(fmt.Sprintf("osq-gsettings-metadata-%s", schema))
	if err != nil {
		return descriptions, fmt.Errorf("mktemp: %w", err)
	}
	defer os.RemoveAll(dir)

	if err := os.Chmod(dir, 0755); err != nil {
		return descriptions, fmt.Errorf("chmod: %w", err)
	}

	keys, err := t.listKeys(ctx, schema, dir)
	if err != nil {
		return descriptions, fmt.Errorf("fetching keys to describe: %w", err)
	}

	for _, k := range keys {
		desc, err := t.describeKey(ctx, schema, k, dir)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"error describing key",
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

func (t *GsettingsMetadata) listKeys(ctx context.Context, schema, tmpdir string) ([]string, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	var keys []string
	output := new(bytes.Buffer)

	err := t.cmdRunner(ctx, []string{"list-keys", schema}, tmpdir, output)
	if err != nil {
		return keys, fmt.Errorf("fetching keys: %w", err)
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
		t.slogger.Log(ctx, slog.LevelInfo,
			"scanner error",
			"err", err,
		)
	}

	return keys, nil
}

// describeKey returns a keyDescription struct that contains metadata about a
// single key, namely a 'description' string/paragraph and an explanation of its
// type
func (t *GsettingsMetadata) describeKey(ctx context.Context, schema, key, tmpdir string) (keyDescription, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	desc := keyDescription{Key: key}

	d, err := t.getDescription(ctx, schema, key, tmpdir)
	if err != nil {
		return desc, fmt.Errorf("getting key's description: %w", err)
	}
	desc.Description = d

	datatype, err := t.getType(ctx, schema, key, tmpdir)
	if err != nil {
		return desc, fmt.Errorf("discerning key's type: %w", err)
	}
	desc.Type = datatype

	return desc, nil
}

func (t *GsettingsMetadata) getDescription(ctx context.Context, schema, key, tmpdir string) (string, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	output := new(bytes.Buffer)

	err := t.cmdRunner(ctx, []string{"describe", schema, key}, tmpdir, output)
	if err != nil {
		return "", fmt.Errorf("describing key: %w", err)
	}

	return strings.TrimSpace(output.String()), nil
}

// getType fetches the type _as described by the gsettings cli tool_ and
// converts it into something human readable. The conversion of the actual
// GVariant type from 'GVariant code' to golang-ish type descriptions is handled
// by convertType
func (t *GsettingsMetadata) getType(ctx context.Context, schema, key, tmpdir string) (string, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	output := new(bytes.Buffer)

	err := t.cmdRunner(ctx, []string{"range", schema, key}, tmpdir, output)
	if err != nil {
		return "", fmt.Errorf("running 'gsettings range': %w", err)
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

// execGsettingsCommand should be called with a tmpdir that will be cleaned up.
func execGsettingsCommand(ctx context.Context, args []string, tmpdir string, output *bytes.Buffer) error {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	command := args[0]
	if err := tablehelpers.Run(ctx, multislogger.NewNopLogger(), 3, allowedcmd.Gsettings, args, output, io.Discard, tablehelpers.WithDir(tmpdir)); err != nil {
		return fmt.Errorf("execing gsettings: %s: %w", command, err)
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
// GVariant-formatted type string. see
// https://developer.gnome.org/glib/unstable/glib-GVariantType.html for
// documentation. Note that not all types listed in the documentation above are
// supported, for example:
//   - tuples (e.g. tuple of 2 strings `(ss)`)
//   - nested types (e.g.// array of tuples: `a(ss)`)
//
// and other complex types are not supported.
func convertType(typ string) string {
	typ = strings.TrimPrefix(typ, "type ") // remove any leading 'type ', eg in 'type b'
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
