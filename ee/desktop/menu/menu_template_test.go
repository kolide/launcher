package menu

import (
	"fmt"
	"testing"
	"time"

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
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   "Hostname: {{.ServerHostname}}, launcher version: {{.LauncherVersion}}",
			output: "Hostname: localhost, launcher version: 0.0.0",
		},
		{
			name:   "unsupported capability",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   "This capability is {{if hasCapability \"bad capability\"}}supported{{else}}unsupported{{end}}.",
			output: "This capability is unsupported.",
		},
		{
			name:   "relativeTime very soon",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This is starting {{if hasCapability \"relativeTime\"}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(2*time.Minute+30*time.Second).Unix()),
			output: "This is starting very soon.",
		},
		{
			name:   "relativeTime 15 minutes",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This is starting {{if hasCapability \"relativeTime\"}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(15*time.Minute+30*time.Second).Unix()),
			output: "This is starting in 15 minutes.",
		},
		{
			name:   "relativeTime one hour",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This is starting {{if hasCapability \"relativeTime\"}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(1*time.Hour+30*time.Second).Unix()),
			output: "This is starting in about an hour.",
		},
		{
			name:   "relativeTime four hours",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This is starting {{if hasCapability \"relativeTime\"}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(4*time.Hour+30*time.Second).Unix()),
			output: "This is starting in 4 hours.",
		},
		{
			name:   "relativeTime one day",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This is starting {{if hasCapability \"relativeTime\"}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(24*time.Hour+30*time.Second).Unix()),
			output: "This is starting in one day.",
		},
		{
			name:   "relativeTime three days",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This is starting {{if hasCapability \"relativeTime\"}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(3*24*time.Hour+30*time.Second).Unix()),
			output: "This is starting in 3 days.",
		},
		{
			name:   "relativeTime seven days",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This is starting {{if hasCapability \"relativeTime\"}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(7*24*time.Hour+30*time.Second).Unix()),
			output: "This is starting in 7 days.",
		},
		{
			name:   "relativeTime two weeks",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This is starting {{if hasCapability \"relativeTime\"}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(14*24*time.Hour+30*time.Second).Unix()),
			output: "This is starting in 2 weeks.",
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

			tp := NewTemplateParser(tt.td)
			o, err := tp.Parse(tt.text)
			if !tt.expectedErr {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.output, o)
		})
	}
}
