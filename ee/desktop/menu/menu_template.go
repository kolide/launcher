package menu

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/go-kit/kit/log"
)

type TemplateData struct {
	LauncherVersion       string
	LauncherRevision      string
	GoVersion             string
	OsqueryVersion        string
	ServerHostname        string
	LogFilePath           string
	LauncherFlagsFilePath string
}

type templateParser struct {
	logger   log.Logger
	filePath string
	td       *TemplateData
}

func NewTemplateParser(logger log.Logger, td *TemplateData) *templateParser {
	tp := &templateParser{
		logger: logger,
		td:     td,
	}

	return tp
}

func (tp *templateParser) parse(text string) (string, error) {
	if tp == nil || tp.td == nil {
		return "", fmt.Errorf("templateData is nil")
	}

	t, err := template.New("menu_template").Parse(text)
	if err != nil {
		return "", fmt.Errorf("could not parse menu template: %w", err)
	}

	var b strings.Builder
	err = t.Execute(&b, tp.td)
	if err != nil {
		return "", fmt.Errorf("could not write menu template output: %w", err)
	}

	return b.String(), nil
}
