package flatpak_upgradeable

import (
	"bytes"
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
)

//go:embed test-data/deb.txt
var deb_data []byte

//go:embed test-data/rhel.txt
var rhel_data []byte

func TestParse(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name     string
		input    []byte
		expected []map[string]string
	}{
		{
			name:     "empty input",
			expected: make([]map[string]string, 0),
		},
		{
			name:  "malformed input",
			input: []byte("\n\nGhdj%\n%@&-gh\t\nfoo   \t    bar\norg.something\n"),
			expected: []map[string]string{
				{
					"id": "org.something",
				},
			},
		},
		{
			name:  "deb_data",
			input: deb_data,
			expected: []map[string]string{
				{
					"id": "com.discordapp.Discord",
				},
				{
					"id": "org.freedesktop.Platform.GL.default",
				},
				{
					"id": "org.freedesktop.Platform.GL.default",
				},
				{
					"id": "org.freedesktop.Platform.Locale",
				},
				{
					"id": "org.freedesktop.Platform",
				},
			},
		},
		{
			name:  "rhel_data",
			input: rhel_data,
			expected: []map[string]string{
				{
					"id": "io.appflowy.AppFlowy",
				},
				{
					"id": "org.gnome.Platform",
				},
				{
					"id": "org.gnome.Platform.Locale",
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := New()
			result, err := p.Parse(bytes.NewReader(tt.input))
			require.NoError(t, err, "unexpected error parsing input")

			require.ElementsMatch(t, tt.expected, result)
		})
	}
}
