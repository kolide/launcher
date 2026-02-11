package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func Test_runSpecs(t *testing.T) {
	t.Parallel()

	ms := multislogger.New()
	err := runSpecs(ms, []string{})
	require.NoError(t, err)
}

func Test_runSpecs_debugFlag(t *testing.T) {
	t.Parallel()

	ms := multislogger.New()
	err := runSpecs(ms, []string{"-quiet", "-debug"})
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
	err := runSpecs(ms, []string{"-quiet", "-required", "nope"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown required field")
	require.Contains(t, err.Error(), "nope")
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
