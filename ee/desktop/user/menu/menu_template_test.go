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
			output: "Version: <no value>",
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
			text:   "This capability is {{if hasCapability `bad capability`}}supported{{else}}unsupported{{end}}.",
			output: "This capability is unsupported.",
		},
		{
			name:   "circleDot capability",
			td:     &TemplateData{},
			text:   "\"icon\":\"{{if not (hasCapability `circleDot`)}}triangle-exclamation{{else}}circle-dot{{end}}\"",
			output: "\"icon\":\"circle-dot\"",
		},
		{
			name:   "icon fallback capability",
			td:     &TemplateData{},
			text:   "\"icon\":\"{{if not (hasCapability `asOfYetUnknownIconType`)}}triangle-exclamation{{else}}new-icon-type{{end}}\"",
			output: "\"icon\":\"triangle-exclamation\"",
		},
		{
			name:   "relativeTime 2 hours ago",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-2 * time.Hour).Unix()},
			text:   "This Menu Was Last Updated {{if hasCapability `relativeTime`}}{{relativeTime .LastMenuUpdateTime}}{{else}}never{{end}}.",
			output: "This Menu Was Last Updated 2 Hours Ago.",
		},
		{
			name:   "relativeTime 15 minutes ago",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-15*time.Minute - 30*time.Second).Unix()},
			text:   "This Menu Was Last Updated {{if hasCapability `relativeTime`}}{{relativeTime .LastMenuUpdateTime}}{{else}}never{{end}}.",
			output: "This Menu Was Last Updated 15 Minutes Ago.",
		},
		{
			name:   "relativeTime one minute ago",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-1 * time.Minute).Unix()},
			text:   "This Menu Was Last Updated {{if hasCapability `relativeTime`}}{{relativeTime .LastMenuUpdateTime}}{{else}}never{{end}}.",
			output: "This Menu Was Last Updated One Minute Ago.",
		},
		{
			name:   "relativeTime one second",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Add(-1 * time.Second).Unix()},
			text:   "This Menu Was Last Updated {{if hasCapability `relativeTime`}}{{relativeTime .LastMenuUpdateTime}}{{else}}never{{end}}.",
			output: "This Menu Was Last Updated Just Now.",
		},
		{
			name:   "relativeTime just now",
			td:     &TemplateData{LastMenuUpdateTime: time.Now().Unix()},
			text:   "This Menu Was Last Updated {{if hasCapability `relativeTime`}}{{relativeTime .LastMenuUpdateTime}}{{else}}never{{end}}.",
			output: "This Menu Was Last Updated Just Now.",
		},
		{
			name:   "relativeTime very soon",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(2*time.Minute+30*time.Second).Unix()),
			output: "This Is Starting Very Soon.",
		},
		{
			name:   "relativeTime 15 minutes",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(15*time.Minute+30*time.Second).Unix()),
			output: "This Is Starting In 15 Minutes.",
		},
		{
			name:   "relativeTime one hour",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(1*time.Hour+30*time.Second).Unix()),
			output: "This Is Starting In About An Hour.",
		},
		{
			name:   "relativeTime 111 minutes",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(111*time.Minute+30*time.Second).Unix()),
			output: "This Is Starting In 111 Minutes.",
		},
		{
			name:   "relativeTime four hours",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(4*time.Hour+30*time.Second).Unix()),
			output: "This Is Starting In 4 Hours.",
		},
		{
			name:   "relativeTime one day",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(24*time.Hour+30*time.Second).Unix()),
			output: "This Is Starting In One Day.",
		},
		{
			name:   "relativeTime 44 hours",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(44*time.Hour+30*time.Second).Unix()),
			output: "This Is Starting In 44 Hours.",
		},
		{
			name:   "relativeTime three days",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(3*24*time.Hour+30*time.Second).Unix()),
			output: "This Is Starting In 3 Days.",
		},
		{
			name:   "relativeTime seven days",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(7*24*time.Hour+30*time.Second).Unix()),
			output: "This Is Starting In 7 Days.",
		},
		{
			name:   "relativeTime two weeks",
			td:     &TemplateData{ServerHostname: "localhost", LauncherVersion: "0.0.0"},
			text:   fmt.Sprintf("This Is Starting {{if hasCapability `relativeTime`}}{{relativeTime %d}}{{else}}never{{end}}.", time.Now().Add(14*24*time.Hour+30*time.Second).Unix()),
			output: "This Is Starting In 2 Weeks.",
		},
		{
			name:   "errorlessTemplateVars",
			td:     &TemplateData{MenuVersion: CurrentMenuVersion},
			text:   "{{if hasCapability `errorlessTemplateVars`}}{{.MenuVersion}}{{else}}{{end}}",
			output: CurrentMenuVersion,
		},
		{
			name:        "unparseable error",
			td:          &TemplateData{},
			text:        "Version: {{GARBAGE}}",
			output:      "",
			expectedErr: true, // An invalid template format is one of the only times errors are expected
		},
		{
			name:   "undefined key",
			td:     &TemplateData{},
			text:   "Version: {{.UndefinedKey}}",
			output: "Version: <no value>",
		},
		{
			name:   "undefined key with value check",
			td:     &TemplateData{},
			text:   "{{if .UndefinedKey}}UndefinedKey is: {{.UndefinedKey}}{{else}}UndefinedKey is NOT set.{{end}}",
			output: "UndefinedKey is NOT set.",
		},
		{
			name:   "current menu version",
			td:     &TemplateData{MenuVersion: CurrentMenuVersion},
			text:   "{{if .MenuVersion}}Menu Version is: {{.MenuVersion}}{{else}}{{end}}",
			output: fmt.Sprintf("Menu Version is: %s", CurrentMenuVersion),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tp := NewTemplateParser(tt.td)
			o, err := tp.Parse(tt.text)
			if tt.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.output, o)
		})
	}
}

func Test_Parse_Seconds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		td           *TemplateData
		text         string
		outputFormat string
		seconds      int
	}{
		{
			name:         "relativeTime 111 seconds ago",
			td:           &TemplateData{LastMenuUpdateTime: time.Now().Add(-111 * time.Second).Unix()},
			text:         "This Menu Was Last Updated {{if hasCapability `relativeTime`}}{{relativeTime .LastMenuUpdateTime}}{{else}}never{{end}}.",
			outputFormat: "This Menu Was Last Updated %d Seconds Ago.",
			seconds:      111,
		},
		{
			name:         "relativeTime 7 seconds ago",
			td:           &TemplateData{LastMenuUpdateTime: time.Now().Add(-7 * time.Second).Unix()},
			text:         "This Menu Was Last Updated {{if hasCapability `relativeTime`}}{{relativeTime .LastMenuUpdateTime}}{{else}}never{{end}}.",
			outputFormat: "This Menu Was Last Updated %d Seconds Ago.",
			seconds:      7,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tp := NewTemplateParser(tt.td)
			o, err := tp.Parse(tt.text)
			require.NoError(t, err)

			// Sometimes we're off by a second and that's fine.
			expectedOutput := fmt.Sprintf(tt.outputFormat, tt.seconds)
			expectedOutputPlusOneSecond := fmt.Sprintf(tt.outputFormat, tt.seconds+1)
			expectedOutputMinusOneSecond := fmt.Sprintf(tt.outputFormat, tt.seconds-1)
			require.True(t,
				o == expectedOutput ||
					o == expectedOutputPlusOneSecond ||
					o == expectedOutputMinusOneSecond,
				fmt.Sprintf("expected output %s to be within one second of %d but was not", o, tt.seconds),
			)
		})
	}
}
