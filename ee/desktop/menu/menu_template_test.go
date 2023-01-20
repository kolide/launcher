package menu

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Parse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		opts        []TemplateDataOption
		text        string
		output      string
		expectedErr bool
	}{
		{
			name: "nil",
		},
		{
			name:   "default",
			opts:   []TemplateDataOption{},
			text:   "Version: {{.LauncherVersion}}",
			output: "Version: unknown",
		},
		{
			name:   "single option",
			opts:   []TemplateDataOption{WithHostname("localhost")},
			text:   "Hostname: {{.Hostname}}",
			output: "Hostname: localhost",
		},
		{
			name:   "multiple option",
			opts:   []TemplateDataOption{WithHostname("localhost"), WithLauncherFlagsFilePath("/launcher.flags")},
			text:   "Hostname: {{.Hostname}}, Launcher flags file: {{.LauncherFlagsFilePath}}",
			output: "Hostname: localhost, Launcher flags file: /launcher.flags",
		},
		{
			name:        "undefined",
			opts:        []TemplateDataOption{},
			text:        "Version: {{GARBAGE}}",
			output:      "",
			expectedErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			td := NewTemplateData(tt.opts...)
			o, err := td.parse(tt.text)
			if !tt.expectedErr {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.output, o)
		})
	}
}
