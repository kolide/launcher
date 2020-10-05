package gsettings

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
)

func TestParser(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		input    string
		expected []map[string]string
	}{
		{
			input:    "blank.txt",
			expected: []map[string]string{},
		},
		{
			input: "simple.txt",
			expected: []map[string]string{{
				"domain": "org.gnome.rhythmbox.plugins.webremote",
				"key":    "access-key",
				"value":  "''",
			},
				{
					"domain": "org.gnome.rhythmbox.plugins.webremote",
					"key":    "foo-bar",
					"value":  "2",
				},
			},
		},
	}

	for _, tt := range tests {
		table := GsettingsValues{logger: log.NewNopLogger()}
		t.Run(tt.input, func(t *testing.T) {
			inputBytes, err := ioutil.ReadFile(filepath.Join("testdata", tt.input))
			require.NoError(t, err, "read file %s", tt.input)

			inputBuffer := bytes.NewBuffer(inputBytes)

			results := []map[string]string{}
			for _, row := range table.parse(inputBuffer) {
				results = append(results, row)
			}

			require.EqualValues(t, tt.expected, results)
		})
	}
}
