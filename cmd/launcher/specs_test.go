package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	osquerytable "github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/require"
)

func Test_runSpecs(t *testing.T) {
	t.Parallel()

	ms := multislogger.New()
	err := runSpecs(ms, []string{"-quiet"})
	require.NoError(t, err)
}

func Test_runSpecs_requiredFlag(t *testing.T) {
	t.Parallel()

	ms := multislogger.New()
	err := runSpecs(ms, []string{"-quiet", "-required", "name", "-required", "columns"})
	require.NoError(t, err)
}

func Test_runSpecs_requiredFlag_unknownField(t *testing.T) {
	t.Parallel()

	ms := multislogger.New()
	err := runSpecs(ms, []string{"-quiet", "-required", "unknownfield"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown field")
	require.Contains(t, err.Error(), "unknownfield")
}

func Test_runSpecs_outputFlag(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "specs.json")

	ms := multislogger.New()
	err := runSpecs(ms, []string{"-output", outPath})
	require.NoError(t, err)

	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Greater(t, len(lines), 0, "output file should contain at least one spec line")
	// Each line should be valid JSON (a table spec object)
	for _, line := range lines {
		if line == "" {
			continue
		}
		require.True(t, strings.HasPrefix(line, "{"), "expected JSON object line: %q", line)
		require.True(t, strings.HasSuffix(line, "}"), "expected JSON object line: %q", line)
	}
}

func Test_runMergeSpecs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Each platform emits NDJSON (one OsqueryTableSpec per line). table_shared
	// appears on all three platforms with differing single-element platform
	// lists; the platform-specific tables appear only in their own file.
	darwin := filepath.Join(dir, "darwin.json")
	require.NoError(t, os.WriteFile(darwin, []byte(strings.Join([]string{
		`{"name":"table_shared","description":"shared","platforms":["darwin"],"columns":[{"name":"c","type":"text"}]}`,
		`{"name":"table_darwin","description":"mac only","platforms":["darwin"],"columns":[]}`,
	}, "\n")+"\n"), 0644))

	linux := filepath.Join(dir, "linux.json")
	require.NoError(t, os.WriteFile(linux, []byte(strings.Join([]string{
		`{"name":"table_shared","description":"shared","platforms":["linux"],"columns":[{"name":"c","type":"text"}]}`,
		`{"name":"table_linux","description":"linux only","platforms":["linux"],"columns":[]}`,
	}, "\n")+"\n"), 0644))

	windows := filepath.Join(dir, "windows.json")
	require.NoError(t, os.WriteFile(windows, []byte(strings.Join([]string{
		`{"name":"table_shared","description":"shared","platforms":["windows"],"columns":[{"name":"c","type":"text"}]}`,
		`{"name":"table_windows","description":"windows only","platforms":["windows"],"columns":[]}`,
	}, "\n")+"\n"), 0644))

	outPath := filepath.Join(dir, "launcher-schema.json")
	require.NoError(t, runMergeSpecs([]string{darwin, linux, windows}, outPath))

	data, err := os.ReadFile(outPath)
	require.NoError(t, err)

	// Output must be a single JSON array (the shape k2 ingests), not NDJSON.
	var combined []osquerytable.OsqueryTableSpec
	require.NoError(t, json.Unmarshal(data, &combined), "merged output should be a JSON array")

	byName := make(map[string]osquerytable.OsqueryTableSpec, len(combined))
	names := make([]string, 0, len(combined))
	for _, spec := range combined {
		byName[spec.Name] = spec
		names = append(names, spec.Name)
	}

	// Deduplicated: table_shared collapses to a single entry across 3 files.
	require.Len(t, combined, 4)
	require.ElementsMatch(t, []string{"table_darwin", "table_linux", "table_shared", "table_windows"}, names)

	// Platforms unioned for the shared table.
	sharedPlatforms := make([]string, 0, len(byName["table_shared"].Platforms))
	for _, p := range byName["table_shared"].Platforms {
		sharedPlatforms = append(sharedPlatforms, string(p))
	}
	require.ElementsMatch(t, []string{"darwin", "linux", "windows"}, sharedPlatforms)

	// Platform-specific tables retain their single platform.
	require.Equal(t, []string{"linux"}, platformStrings(byName["table_linux"].Platforms))

	// Sorted by name.
	require.True(t, sortedStrings(names), "combined tables should be sorted by name: %v", names)
}

func Test_runMergeSpecs_noInputs(t *testing.T) {
	t.Parallel()

	err := runMergeSpecs(nil, "")
	require.Error(t, err)
}

func Test_runMergeSpecs_schemaMismatch_columnPresence(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// table_shared has an extra "b" column on darwin that linux does not have.
	darwin := filepath.Join(dir, "darwin.json")
	require.NoError(t, os.WriteFile(darwin, []byte(
		`{"name":"table_shared","description":"shared","platforms":["darwin"],"columns":[{"name":"a","type":"text"},{"name":"b","type":"text"}]}`+"\n"), 0644))

	linux := filepath.Join(dir, "linux.json")
	require.NoError(t, os.WriteFile(linux, []byte(
		`{"name":"table_shared","description":"shared","platforms":["linux"],"columns":[{"name":"a","type":"text"}]}`+"\n"), 0644))

	outPath := filepath.Join(dir, "launcher-schema.json")
	err := runMergeSpecs([]string{darwin, linux}, outPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "schema mismatch")
	require.Contains(t, err.Error(), "table_shared")
	require.Contains(t, err.Error(), `"b"`)

	// The conflicting schema must not be written out.
	_, statErr := os.Stat(outPath)
	require.True(t, os.IsNotExist(statErr), "no output file should be written when a schema conflict is detected")
}

func Test_runMergeSpecs_schemaMismatch_columnType(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// table_shared.c is text on darwin but integer on windows.
	darwin := filepath.Join(dir, "darwin.json")
	require.NoError(t, os.WriteFile(darwin, []byte(
		`{"name":"table_shared","description":"shared","platforms":["darwin"],"columns":[{"name":"c","type":"text"}]}`+"\n"), 0644))

	windows := filepath.Join(dir, "windows.json")
	require.NoError(t, os.WriteFile(windows, []byte(
		`{"name":"table_shared","description":"shared","platforms":["windows"],"columns":[{"name":"c","type":"integer"}]}`+"\n"), 0644))

	err := runMergeSpecs([]string{darwin, windows}, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "schema mismatch")
	require.Contains(t, err.Error(), "table_shared")
	require.Contains(t, err.Error(), `"c"`)
}

func Test_schemaConflicts(t *testing.T) {
	t.Parallel()

	col := func(name, ctype string) osquerytable.ColumnDefinition {
		return osquerytable.ColumnDefinition{Name: name, Type: osquerytable.ColumnType(ctype)}
	}

	tests := []struct {
		name     string
		a        []osquerytable.ColumnDefinition
		b        []osquerytable.ColumnDefinition
		wantSame bool
	}{
		{
			name:     "identical columns",
			a:        []osquerytable.ColumnDefinition{col("a", "text"), col("b", "integer")},
			b:        []osquerytable.ColumnDefinition{col("a", "text"), col("b", "integer")},
			wantSame: true,
		},
		{
			name:     "reordered columns still match",
			a:        []osquerytable.ColumnDefinition{col("a", "text"), col("b", "integer")},
			b:        []osquerytable.ColumnDefinition{col("b", "integer"), col("a", "text")},
			wantSame: true,
		},
		{
			name:     "column missing on one side",
			a:        []osquerytable.ColumnDefinition{col("a", "text"), col("b", "integer")},
			b:        []osquerytable.ColumnDefinition{col("a", "text")},
			wantSame: false,
		},
		{
			name:     "differing column type",
			a:        []osquerytable.ColumnDefinition{col("a", "text")},
			b:        []osquerytable.ColumnDefinition{col("a", "integer")},
			wantSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conflicts := schemaConflicts(
				osquerytable.OsqueryTableSpec{Name: "t", Columns: tt.a},
				osquerytable.OsqueryTableSpec{Name: "t", Columns: tt.b},
			)
			if tt.wantSame {
				require.Empty(t, conflicts)
			} else {
				require.NotEmpty(t, conflicts)
			}
		})
	}
}

func platformStrings[T ~string](platforms []T) []string {
	out := make([]string, 0, len(platforms))
	for _, p := range platforms {
		out = append(out, string(p))
	}
	return out
}

func sortedStrings(s []string) bool {
	for i := 1; i < len(s); i++ {
		if s[i-1] > s[i] {
			return false
		}
	}
	return true
}
