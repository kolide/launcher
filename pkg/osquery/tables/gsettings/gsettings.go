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
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

type Table struct {
	client *osquery.ExtensionManagerClient
	logger log.Logger
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		// TODO: maybe need to add 'path' for relocatable schemas..
		table.TextColumn("domain"),
		table.TextColumn("key"),
		table.TextColumn("type"),
		table.TextColumn("value"),
		table.TextColumn("description"),
	}

	t := &Table{
		client: client,
		logger: logger,
	}

	return table.NewPlugin("kolide_gsettings", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	domainQ, ok := queryContext.Constraints["domain"]
	if !ok || len(domainQ.Constraints) == 0 {
		return nil, errors.New("the kolide_gsettings table requires a domain to be specified")
	}

	for _, domainConstraint := range domainQ.Constraints {
		// TODO: this doesn't handle wildcards or partial matches..
		osqueryResults, err := t.gsettings(ctx, domainConstraint.Expression)
		if err != nil {
			continue
		}

		for _, row := range osqueryResults {
			results = append(results, row)
		}
	}
	return results, nil
}

func (t *Table) gsettings(ctx context.Context, domain string) ([]map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/usr/bin/gsettings", "list-recursively", domain)
	dir, err := ioutil.TempDir("", "osq-gsettings")
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
		level.Info(t.logger).Log(
			"msg", "Error running gsettings",
			"stderr", strings.TrimSpace(stderr.String()),
			"stdout", strings.TrimSpace(stdout.String()),
			"err", err,
		)
		return nil, errors.Wrap(err, "running osquery")
	}
	osqueryResults := t.parse(stdout)
	for _, row := range osqueryResults {
		d, err := t.gsettingsDescribe(ctx, row["key"], row["domain"])
		if err != nil {
			continue
		}
		row["description"] = d.Description
		row["type"] = d.Type
	}

	return osqueryResults, nil
}

type datatype struct {
	raw string
}

type keyDescription struct {
	Description string
	Type        string
}

func (t *Table) gsettingsDescribe(ctx context.Context, key, domain string) (keyDescription, error) {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	var desc keyDescription
	cmd := exec.CommandContext(ctx, "/usr/bin/gsettings", "describe", domain, key)
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
			"domain", domain,
			"stderr", strings.TrimSpace(stderr.String()),
			"stdout", strings.TrimSpace(stdout.String()),
			"err", err,
		)
		return keyDescription{}, errors.Wrap(err, "running gsettings describe")
	}
	desc.Description = strings.TrimSpace(stdout.String())
	datatype, err := t.getType(ctx, key, domain)
	if err != nil {
		return desc, errors.Wrap(err, "discerning key's type")
	}
	desc.Type = datatype

	return desc, nil
}

func (t *Table) getType(ctx context.Context, key, domain string) (string, error) {
	cmd := exec.CommandContext(ctx, "/usr/bin/gsettings", "range", domain, key)
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
			"domain", domain,
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
	/*
		b: the type string of G_VARIANT_TYPE_BOOLEAN; a boolean value.
		y: the type string of G_VARIANT_TYPE_BYTE; a byte.
		n: the type string of G_VARIANT_TYPE_INT16; a signed 16 bit integer.
		q: the type string of G_VARIANT_TYPE_UINT16; an unsigned 16 bit integer.
		i: the type string of G_VARIANT_TYPE_INT32; a signed 32 bit integer.
		u: the type string of G_VARIANT_TYPE_UINT32; an unsigned 32 bit integer.
		x: the type string of G_VARIANT_TYPE_INT64; a signed 64 bit integer.
		t: the type string of G_VARIANT_TYPE_UINT64; an unsigned 64 bit integer.
		h: the type string of G_VARIANT_TYPE_HANDLE; a signed 32 bit value that, by convention, is used as an index into an array of file descriptors that are sent alongside a D-Bus message.
		d: the type string of G_VARIANT_TYPE_DOUBLE; a double precision floating point value.
		s: the type string of G_VARIANT_TYPE_STRING; a string.
		a: used as a prefix on another type string to mean an array of that type; the type string "ai", for example, is the type of an array of signed 32-bit integers.
	*/
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

func (t *Table) parse(input *bytes.Buffer) []map[string]string {
	var results []map[string]string

	scanner := bufio.NewScanner(input)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		row := make(map[string]string)
		parts := strings.SplitN(line, " ", 3)
		row["domain"] = parts[0]
		row["key"] = parts[1]
		row["value"] = parts[2]
		results = append(results, row)
	}

	if err := scanner.Err(); err != nil {
		level.Debug(t.logger).Log("msg", "scanner error", "err", err)
	}

	return results
}
