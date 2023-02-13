package menu

import (
	"fmt"
	"strings"
	"text/template"
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

	t, err := template.New("menu_template").Parse(text)
	if err != nil {
		return "", fmt.Errorf("could not parse template: %w", err)
	}

	var b strings.Builder
	if err := t.Execute(&b, tp.td) && err != nil {
		return "", fmt.Errorf("could not write template output: %w", err)
	}

	return b.String(), nil
}
