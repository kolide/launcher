package menu

import (
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/xeonx/timeago"
)

const (
	funcHasCapability = "hasCapability"
	funcRelativeTime  = "relativeTime"
)

type TemplateData struct {
	LauncherVersion  string `json:",omitempty"`
	LauncherRevision string `json:",omitempty"`
	GoVersion        string `json:",omitempty"`
	ServerHostname   string `json:",omitempty"`
}

type templateParser struct {
	td *TemplateData
}

func NewTemplateParser(td *TemplateData) *templateParser {
	tp := &templateParser{
		td: td,
	}

	return tp
}

// Parse parses text as a template body for the menu template data
// if an error occurs while parsing, an empty string is returned along with the error
func (tp *templateParser) Parse(text string) (string, error) {
	if tp == nil || tp.td == nil {
		return "", fmt.Errorf("templateData is nil")
	}

	t, err := template.New("menu_template").Funcs(template.FuncMap{
		// hasCapability enables interoperability between different versions of launcher
		funcHasCapability: func(capability string) bool {
			if capability == funcRelativeTime {
				return true
			}
			return false
		},
		// relativeTime takes a RFC 339 formatted timestamp and returns a fuzzy timestamp
		funcRelativeTime: func(timestamp string) string {
			t, err := time.Parse(
				time.RFC3339,
				timestamp)
			if err != nil {
				return ""
			}

			return timeago.NoMax(timeago.English).Format(t)
		},
	}).Parse(text)
	if err != nil {
		return "", fmt.Errorf("could not parse template: %w", err)
	}

	var b strings.Builder
	if err := t.Execute(&b, tp.td); err != nil {
		return "", fmt.Errorf("could not write template output: %w", err)
	}

	return b.String(), nil
}
