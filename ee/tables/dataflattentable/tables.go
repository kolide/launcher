package dataflattentable

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/kolide/launcher/v2/ee/dataflatten"
	"github.com/kolide/launcher/v2/ee/observability"
	"github.com/kolide/launcher/v2/ee/tables/tablehelpers"
	"github.com/kolide/launcher/v2/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
)

type DataSourceType struct {
	tableName   string
	description string

	// Required: factory returning the bytes flatten func. Receives QueryContext
	// so types can vary behavior per-query.
	flattenBytesFunc func(table.QueryContext) dataflatten.DataFunc

	// Optional: factory returning the file flatten func. When nil, the table
	// auto-generates one that reads the file with os.ReadFile and delegates
	// to flattenBytesFunc. Only needed when file handling differs from
	// "read bytes, parse bytes" (e.g., JSON's UTF-16 fallback, XML's reader API).
	flattenFileFunc func(table.QueryContext) dataflatten.DataFileFunc

	// Optional: When not nil, these extra columns are included in the schema.
	// This is required to be used if a table consumes data from the QueryContext.
	extraColumns []table.ColumnDefinition
}

var allTypes = []DataSourceType{
	{
		tableName:        "kolide_json",
		description:      "Parses JSON files or raw JSON data and returns flattened key-value pairs. Requires a WHERE path = or raw_data = constraint. Supports a query constraint for filtering specific keys. Useful for reading any JSON configuration or data file.",
		flattenBytesFunc: func(_ table.QueryContext) dataflatten.DataFunc { return dataflatten.Json },
		flattenFileFunc:  func(_ table.QueryContext) dataflatten.DataFileFunc { return dataflatten.JsonFile },
	},
	{
		tableName:        "kolide_jsonc",
		description:      "Parses JSONC files or raw JSONC data and returns flattened key-value pairs. Requires a WHERE path = or raw_data = constraint. Supports a query constraint for filtering specific keys. Useful for reading any JSONC configuration or data file.",
		flattenBytesFunc: func(_ table.QueryContext) dataflatten.DataFunc { return dataflatten.Jsonc },
		flattenFileFunc:  func(_ table.QueryContext) dataflatten.DataFileFunc { return dataflatten.JsoncFile },
	},
	{
		tableName:        "kolide_xml",
		description:      "Parses XML files or raw XML data and returns flattened key-value pairs. Requires a WHERE path = or raw_data = constraint. Supports a query constraint for filtering specific keys. Useful for reading XML configuration or data files.",
		flattenBytesFunc: func(_ table.QueryContext) dataflatten.DataFunc { return dataflatten.Xml },
		flattenFileFunc:  func(_ table.QueryContext) dataflatten.DataFileFunc { return dataflatten.XmlFile },
	},
	{
		tableName:        "kolide_ini",
		description:      "Parses INI files or raw INI data and returns flattened key-value pairs. Requires a WHERE path = or raw_data = constraint. Supports a query constraint for filtering specific keys. Useful for reading INI-style configuration files.",
		flattenBytesFunc: func(_ table.QueryContext) dataflatten.DataFunc { return dataflatten.Ini },
		flattenFileFunc:  func(_ table.QueryContext) dataflatten.DataFileFunc { return dataflatten.IniFile },
	},
	{
		tableName:        "kolide_plist",
		description:      "Parses Apple plist files or raw plist data and returns flattened key-value pairs. Requires a WHERE path = or raw_data = constraint. Supports a query constraint for filtering specific keys. Useful for reading macOS preference files, application plists, and system configuration.",
		flattenBytesFunc: func(_ table.QueryContext) dataflatten.DataFunc { return dataflatten.Plist },
	},
	{
		tableName:        "kolide_jsonl",
		description:      "Parses JSONL (JSON Lines) files or raw data and returns flattened key-value pairs. Requires a WHERE path = or raw_data = constraint. Supports a query constraint for filtering specific keys. Useful for reading line-delimited JSON log files.",
		flattenBytesFunc: func(_ table.QueryContext) dataflatten.DataFunc { return dataflatten.Jsonl },
	},
	{
		tableName:        "kolide_protobuf",
		description:      "Parses marshaled protobuf files or raw protobuf data and returns flattened key-value pairs. Field numbers are used as keys since protobuf wire format is schema-less. Requires a WHERE path = or raw_data = constraint.",
		flattenBytesFunc: func(_ table.QueryContext) dataflatten.DataFunc { return dataflatten.Protobuf },
	},
	{
		flattenBytesFunc: func(_ table.QueryContext) dataflatten.DataFunc { return dataflatten.Toml },
		flattenFileFunc:  func(_ table.QueryContext) dataflatten.DataFileFunc { return dataflatten.TomlFile },
		tableName:        "kolide_toml",
		description:      "Parses TOML files or raw TOML data and returns flattened key-value pairs. Requires a WHERE path = or raw_data = constraint. Supports a query constraint for filtering specific keys. Useful for reading TOML configuration files (e.g. Cargo.toml, pyproject.toml).",
	},
	{
		tableName:        "kolide_yaml",
		description:      "Parses YAML files or raw YAML data and returns flattened key-value pairs. Requires a WHERE path = or raw_data = constraint. Supports a query constraint for filtering specific keys. Useful for reading any YAML configuration or data file.",
		flattenBytesFunc: func(_ table.QueryContext) dataflatten.DataFunc { return dataflatten.Yaml },
		flattenFileFunc:  func(_ table.QueryContext) dataflatten.DataFileFunc { return dataflatten.YamlFile },
	},
}

type Table struct {
	slogger   *slog.Logger
	tableName string

	flattenBytesFunc func(table.QueryContext) dataflatten.DataFunc
	flattenFileFunc  func(table.QueryContext) dataflatten.DataFileFunc
}

// AllTablePlugins is a helper to return all the expected flattening tables.
func AllTablePlugins(flags types.Flags, slogger *slog.Logger) []osquery.OsqueryPlugin {
	plugins := make([]osquery.OsqueryPlugin, 0, len(allTypes))
	for _, dst := range allTypes {
		plugins = append(plugins, TablePlugin(flags, slogger, dst))
	}
	return plugins
}

// ArchiveMemberNote documents the member constraint supported by the dataflatten tables.
const ArchiveMemberNote = "When a 'member' constraint is provided alongside 'path', the path is treated as a zip-format archive (zip, jar, etc) and the matching member files are extracted and parsed. Member names support SQL-style % wildcards (e.g. WHERE path = '/path/to/plugin.jar' AND member = 'META-INF/plugin.xml')."

func TablePlugin(flags types.Flags, slogger *slog.Logger, dst DataSourceType) osquery.OsqueryPlugin {
	columns := Columns(append(
		[]table.ColumnDefinition{table.TextColumn("path"), table.TextColumn("raw_data"), table.TextColumn("member")},
		dst.extraColumns...,
	)...)

	t := &Table{
		slogger:          slogger.With("table", dst.tableName),
		tableName:        dst.tableName,
		flattenBytesFunc: dst.flattenBytesFunc,
		flattenFileFunc:  dst.flattenFileFunc,
	}

	var opts []tablewrapper.TablePluginOption
	opts = append(opts, tablewrapper.WithDescription(dst.description))
	opts = append(opts, tablewrapper.WithNote(EAVNote+"\n\n"+ArchiveMemberNote))

	return tablewrapper.New(flags, slogger, dst.tableName, columns, t.generate, opts...)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", t.tableName)
	defer span.End()

	var results []map[string]string

	requestedPaths := tablehelpers.GetConstraints(queryContext, "path")
	requestedRawDatas := tablehelpers.GetConstraints(queryContext, "raw_data")
	requestedMembers := tablehelpers.GetConstraints(queryContext, "member")

	if len(requestedPaths) == 0 && len(requestedRawDatas) == 0 {
		return results, fmt.Errorf("The %s table requires that you specify at least one of 'path' or 'raw_data'", t.tableName)
	}

	if len(requestedMembers) > 0 && len(requestedPaths) == 0 {
		return results, fmt.Errorf("The %s table requires a 'path' constraint when 'member' is specified", t.tableName)
	}

	flattenOpts := []dataflatten.FlattenOpts{
		dataflatten.WithSlogger(t.slogger),
		dataflatten.WithNestedPlist(),
	}

	for _, requestedPath := range requestedPaths {

		// We take globs in via the sql %, but glob needs *. So convert.
		filePaths, err := filepath.Glob(strings.ReplaceAll(requestedPath, `%`, `*`))
		if err != nil {
			return results, fmt.Errorf("bad glob: %w", err)
		}

		for _, filePath := range filePaths {
			for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
				var subresults []map[string]string
				var err error
				if len(requestedMembers) > 0 {
					subresults, err = t.generateArchiveMembers(ctx, queryContext, filePath, requestedMembers, dataQuery, append(flattenOpts, dataflatten.WithQuery(strings.Split(dataQuery, "/")))...)
				} else {
					subresults, err = t.generatePath(ctx, queryContext, filePath, dataQuery, append(flattenOpts, dataflatten.WithQuery(strings.Split(dataQuery, "/")))...)
				}
				if err != nil {
					t.slogger.Log(ctx, slog.LevelInfo,
						"failed to get data for path",
						"path", filePath,
						"err", err,
					)
					continue
				}

				results = append(results, subresults...)
			}
		}
	}

	for _, rawdata := range requestedRawDatas {
		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			subresults, err := t.generateRawData(ctx, queryContext, rawdata, dataQuery, append(flattenOpts, dataflatten.WithQuery(strings.Split(dataQuery, "/")))...)
			if err != nil {
				t.slogger.Log(ctx, slog.LevelInfo,
					"failed to generate for raw_data",
					"err", err,
				)
				continue
			}

			results = append(results, subresults...)
		}
	}

	return results, nil
}

func (t *Table) generateRawData(ctx context.Context, qc table.QueryContext, rawdata string, dataQuery string, flattenOpts ...dataflatten.FlattenOpts) ([]map[string]string, error) {
	data, err := t.flattenBytesFunc(qc)([]byte(rawdata), flattenOpts...)
	if err != nil {
		t.slogger.Log(ctx, slog.LevelInfo,
			"failure parsing raw data",
			"err", err,
		)
		return nil, fmt.Errorf("parsing data: %w", err)
	}

	rowData := map[string]string{
		"raw_data": rawdata,
	}

	return ToMap(data, dataQuery, rowData), nil
}

func (t *Table) generatePath(ctx context.Context, qc table.QueryContext, filePath string, dataQuery string, flattenOpts ...dataflatten.FlattenOpts) ([]map[string]string, error) {
	var data []dataflatten.Row
	var err error
	if t.flattenFileFunc != nil {
		data, err = t.flattenFileFunc(qc)(filePath, flattenOpts...)
	} else {
		raw, readErr := os.ReadFile(filePath)
		if readErr != nil {
			return nil, fmt.Errorf("reading %s: %w", filePath, readErr)
		}
		data, err = t.flattenBytesFunc(qc)(raw, flattenOpts...)
	}
	if err != nil {
		t.slogger.Log(ctx, slog.LevelInfo,
			"failure parsing file",
			"file", filePath,
		)
		return nil, fmt.Errorf("parsing data: %w", err)
	}

	rowData := map[string]string{
		"path": filePath,
	}

	return ToMap(data, dataQuery, rowData), nil
}

// generateArchiveMembers treats filePath as a zip-format archive (zip, jar,
// etc), extracts each member whose name matches one of requestedMembers, and
// parses it with the table's bytes parser. Member patterns support SQL-style
// % wildcards, matching the glob support on the path column.
func (t *Table) generateArchiveMembers(ctx context.Context, qc table.QueryContext, filePath string, requestedMembers []string, dataQuery string, flattenOpts ...dataflatten.FlattenOpts) ([]map[string]string, error) {
	zipReader, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening %s as zip archive: %w", filePath, err)
	}
	defer zipReader.Close()

	var results []map[string]string

	for _, zipFile := range zipReader.File {
		if zipFile.FileInfo().IsDir() || !anyMemberMatches(requestedMembers, zipFile.Name) {
			continue
		}

		raw, err := readArchiveMember(zipFile)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"failed to read archive member",
				"path", filePath,
				"member", zipFile.Name,
				"err", err,
			)
			continue
		}

		data, err := t.flattenBytesFunc(qc)(raw, flattenOpts...)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"failure parsing archive member",
				"path", filePath,
				"member", zipFile.Name,
				"err", err,
			)
			continue
		}

		rowData := map[string]string{
			"path":   filePath,
			"member": zipFile.Name,
		}

		results = append(results, ToMap(data, dataQuery, rowData)...)
	}

	return results, nil
}

// maxArchiveMemberSize bounds how many uncompressed bytes we will read from a
// single archive member. Archives are highly compressible, so a small file on
// disk can inflate to gigabytes ("zip bomb"); without a cap, parsing a matched
// member would allocate that entire size into memory. The manifests this table
// targets (plugin.xml, package.json) are a few KB, so 32 MiB is generous while
// keeping worst-case memory bounded.
const maxArchiveMemberSize = 32 << 20

func readArchiveMember(zipFile *zip.File) ([]byte, error) {
	// The zip central directory declares each member's uncompressed size, so we
	// can reject an oversized member without inflating a single byte.
	if zipFile.UncompressedSize64 > maxArchiveMemberSize {
		return nil, fmt.Errorf("member %s declares %d bytes, exceeds %d byte cap", zipFile.Name, zipFile.UncompressedSize64, maxArchiveMemberSize)
	}

	rdr, err := zipFile.Open()
	if err != nil {
		return nil, fmt.Errorf("opening member: %w", err)
	}
	defer rdr.Close()

	// The declared size above is attacker-controlled and can lie, so cap the
	// actual read too. Read one byte past the cap to detect an overrun.
	raw, err := io.ReadAll(io.LimitReader(rdr, maxArchiveMemberSize+1))
	if err != nil {
		return nil, fmt.Errorf("reading member: %w", err)
	}
	if len(raw) > maxArchiveMemberSize {
		return nil, fmt.Errorf("member %s exceeds %d byte cap", zipFile.Name, maxArchiveMemberSize)
	}

	return raw, nil
}

func anyMemberMatches(patterns []string, name string) bool {
	for _, pattern := range patterns {
		if memberMatches(pattern, name) {
			return true
		}
	}
	return false
}

// memberMatches reports whether an archive member name matches pattern, where
// % matches any run of characters, including path separators.
func memberMatches(pattern, name string) bool {
	segments := strings.Split(pattern, "%")
	if len(segments) == 1 {
		return pattern == name
	}

	if !strings.HasPrefix(name, segments[0]) {
		return false
	}
	name = name[len(segments[0]):]

	for _, segment := range segments[1 : len(segments)-1] {
		idx := strings.Index(name, segment)
		if idx < 0 {
			return false
		}
		name = name[idx+len(segment):]
	}

	return strings.HasSuffix(name, segments[len(segments)-1])
}
