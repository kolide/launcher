package menu

import (
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Parse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		td          *TemplateData
		text        string
		output      string
		expectedErr bool
	}{
		{
			name:   "default",
			td:     &TemplateData{},
			text:   "Version: {{.LauncherVersion}}",
			output: "Version: ",
		},
		{
			name:   "single option",
			td:     &TemplateData{ServerHostname: "localhost"},
			text:   "Hostname: {{.ServerHostname}}",
			output: "Hostname: localhost",
		},
		{
			name:   "multiple option",
			td:     &TemplateData{ServerHostname: "localhost", OsqueryVersion: "0.0.0"},
			text:   "Hostname: {{.ServerHostname}}, Osquery version: {{.OsqueryVersion}}",
			output: "Hostname: localhost, Osquery version: 0.0.0",
		},
		{
			name:        "undefined",
			td:          &TemplateData{},
			text:        "Version: {{GARBAGE}}",
			output:      "",
			expectedErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tp := NewTemplateParser(log.NewNopLogger(), tt.td)
			o, err := tp.parse(tt.text)
			if !tt.expectedErr {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.output, o)
		})
	}
}
