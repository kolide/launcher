package dataflattentable

import (
	"archive/zip"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/kolide/launcher/v2/ee/dataflatten"
	"github.com/kolide/launcher/v2/ee/tables/tablehelpers"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

const testPluginXml = `<idea-plugin>
  <id>com.intellij.kubernetes</id>
  <name>Kubernetes</name>
  <version>233.11799.196</version>
  <vendor>JetBrains</vendor>
</idea-plugin>
`

const testConfigJson = `{"name": "test-plugin", "enabled": true}`

// writeTestJar creates a small jar (zip) fixture containing a JetBrains-style
// plugin manifest, plus other members that should not match manifest queries.
func writeTestJar(t *testing.T) string {
	t.Helper()

	jarPath := filepath.Join(t.TempDir(), "clouds-kubernetes.jar")
	jarFile, err := os.Create(jarPath)
	require.NoError(t, err)
	defer jarFile.Close()

	zipWriter := zip.NewWriter(jarFile)

	_, err = zipWriter.Create("META-INF/")
	require.NoError(t, err)

	for name, contents := range map[string]string{
		"META-INF/plugin.xml":  testPluginXml,
		"META-INF/MANIFEST.MF": "Manifest-Version: 1.0\n",
		"config.json":          testConfigJson,
	} {
		w, err := zipWriter.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(contents))
		require.NoError(t, err)
	}

	require.NoError(t, zipWriter.Close())

	return jarPath
}

func TestDataFlattenTable_ArchiveMembers(t *testing.T) {
	t.Parallel()

	slogger := multislogger.NewNopLogger()
	jarPath := writeTestJar(t)

	xmlTable := Table{slogger: slogger, tableName: "kolide_xml", flattenBytesFunc: staticBytes(dataflatten.Xml)}
	jsonTable := Table{slogger: slogger, tableName: "kolide_json", flattenBytesFunc: staticBytes(dataflatten.Json)}

	var tests = []struct {
		name     string
		table    Table
		members  []string
		queries  []string
		expected []map[string]string
	}{
		{
			name:    "exact member",
			table:   xmlTable,
			members: []string{"META-INF/plugin.xml"},
			queries: []string{"idea-plugin/version"},
			expected: []map[string]string{
				{"fullkey": "idea-plugin/version", "parent": "idea-plugin", "key": "version", "value": "233.11799.196", "member": "META-INF/plugin.xml"},
			},
		},
		{
			name:    "wildcard member",
			table:   xmlTable,
			members: []string{"%/plugin.xml"},
			queries: []string{"idea-plugin/id"},
			expected: []map[string]string{
				{"fullkey": "idea-plugin/id", "parent": "idea-plugin", "key": "id", "value": "com.intellij.kubernetes", "member": "META-INF/plugin.xml"},
			},
		},
		{
			name:    "json member",
			table:   jsonTable,
			members: []string{"config.json"},
			queries: []string{"name"},
			expected: []map[string]string{
				{"fullkey": "name", "parent": "", "key": "name", "value": "test-plugin", "member": "config.json"},
			},
		},
		{
			name:    "missing member returns no rows",
			table:   xmlTable,
			members: []string{"META-INF/nonexistent.xml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockQC := tablehelpers.MockQueryContext(map[string][]string{
				"path":   {jarPath},
				"member": tt.members,
				"query":  tt.queries,
			})

			rows, err := tt.table.generate(t.Context(), mockQC)
			require.NoError(t, err)

			// delete the path and query keys, so we don't need to enumerate them in the test case
			for _, row := range rows {
				delete(row, "path")
				delete(row, "query")
			}

			sort.SliceStable(rows, func(i, j int) bool { return rows[i]["fullkey"] < rows[j]["fullkey"] })

			if tt.expected == nil {
				require.Empty(t, rows)
			} else {
				require.EqualValues(t, tt.expected, rows)
			}
		})
	}
}

func TestDataFlattenTable_ArchiveMemberErrors(t *testing.T) {
	t.Parallel()

	slogger := multislogger.NewNopLogger()
	xmlTable := Table{slogger: slogger, tableName: "kolide_xml", flattenBytesFunc: staticBytes(dataflatten.Xml)}

	// member without path is an error
	_, err := xmlTable.generate(t.Context(), tablehelpers.MockQueryContext(map[string][]string{
		"raw_data": {testPluginXml},
		"member":   {"META-INF/plugin.xml"},
	}))
	require.Error(t, err)

	// a member constraint against a non-archive file logs and skips, returning no rows
	rows, err := xmlTable.generate(t.Context(), tablehelpers.MockQueryContext(map[string][]string{
		"path":   {filepath.Join("testdata", "animals.xml")},
		"member": {"META-INF/plugin.xml"},
	}))
	require.NoError(t, err)
	require.Empty(t, rows)
}

// writeBombJar creates a jar containing one highly compressible member that
// inflates to just over maxArchiveMemberSize, alongside a normal manifest.
func writeBombJar(t *testing.T) string {
	t.Helper()

	jarPath := filepath.Join(t.TempDir(), "bomb.jar")
	jarFile, err := os.Create(jarPath)
	require.NoError(t, err)
	defer jarFile.Close()

	zipWriter := zip.NewWriter(jarFile)

	w, err := zipWriter.Create("META-INF/plugin.xml")
	require.NoError(t, err)
	_, err = w.Write([]byte(testPluginXml))
	require.NoError(t, err)

	// A run of zeros compresses to almost nothing but inflates past the cap.
	bomb, err := zipWriter.Create("bomb.xml")
	require.NoError(t, err)
	zeros := make([]byte, 1<<20)
	for written := 0; written <= maxArchiveMemberSize; written += len(zeros) {
		_, err = bomb.Write(zeros)
		require.NoError(t, err)
	}

	require.NoError(t, zipWriter.Close())

	return jarPath
}

func TestDataFlattenTable_ArchiveMemberSizeCap(t *testing.T) {
	t.Parallel()

	slogger := multislogger.NewNopLogger()
	jarPath := writeBombJar(t)
	xmlTable := Table{slogger: slogger, tableName: "kolide_xml", flattenBytesFunc: staticBytes(dataflatten.Xml)}

	// The oversized member is rejected and produces no rows, without erroring
	// the query.
	rows, err := xmlTable.generate(t.Context(), tablehelpers.MockQueryContext(map[string][]string{
		"path":   {jarPath},
		"member": {"bomb.xml"},
	}))
	require.NoError(t, err)
	require.Empty(t, rows, "oversized member should be skipped")

	// A normal member in the same archive still parses fine.
	rows, err = xmlTable.generate(t.Context(), tablehelpers.MockQueryContext(map[string][]string{
		"path":   {jarPath},
		"member": {"META-INF/plugin.xml"},
		"query":  {"idea-plugin/version"},
	}))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "233.11799.196", rows[0]["value"])

	// A wildcard that matches both must skip only the bomb, returning the good one.
	rows, err = xmlTable.generate(t.Context(), tablehelpers.MockQueryContext(map[string][]string{
		"path":   {jarPath},
		"member": {"%"},
		"query":  {"idea-plugin/version"},
	}))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "233.11799.196", rows[0]["value"])
}

func TestMemberMatches(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		pattern string
		name    string
		matches bool
	}{
		{"META-INF/plugin.xml", "META-INF/plugin.xml", true},
		{"META-INF/plugin.xml", "META-INF/pluginIcon.svg", false},
		{"%/plugin.xml", "META-INF/plugin.xml", true},
		{"%.xml", "META-INF/plugin.xml", true},
		{"META-INF/%", "META-INF/plugin.xml", true},
		{"META-INF/%", "config.json", false},
		{"%plugin%", "META-INF/plugin.xml", true},
		{"%", "anything/at/all", true},
		{"a%a", "a", false},
		{"a%a", "aa", true},
		{"a%aa%a", "aaa", false},
		{"a%aa%a", "aaaa", true},
	}

	for _, tt := range tests {
		require.Equal(t, tt.matches, memberMatches(tt.pattern, tt.name), "pattern %q against %q", tt.pattern, tt.name)
	}
}
