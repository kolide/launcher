package menu

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/kolide/kit/version"
)

type TemplateDataOption func(*templateData)

func WithVersion(info version.Info) TemplateDataOption {
	return func(td *templateData) {
		td.LauncherVersion = info.Version
		td.LauncherRevision = info.Revision
		td.GoVersion = info.GoVersion
	}
}

func WithOSQueryVersion(osqueryVersion string) TemplateDataOption {
	return func(td *templateData) {
		td.OSQueryVersion = osqueryVersion
	}
}

func WithHostname(hostname string) TemplateDataOption {
	return func(td *templateData) {
		td.Hostname = hostname
	}
}

func WithLogFilePath(logFilePath string) TemplateDataOption {
	return func(td *templateData) {
		td.LogFilePath = logFilePath
	}
}

func WithLauncherFlagsFilePath(launcherFlagsFilePath string) TemplateDataOption {
	return func(td *templateData) {
		td.LauncherFlagsFilePath = launcherFlagsFilePath
	}
}

type templateData struct {
	LauncherVersion       string
	LauncherRevision      string
	GoVersion             string
	OSQueryVersion        string
	Hostname              string
	LogFilePath           string
	LauncherFlagsFilePath string
}

func NewTemplateData(opts ...TemplateDataOption) *templateData {
	template := &templateData{
		LauncherVersion:       "unknown",
		LauncherRevision:      "unknown",
		OSQueryVersion:        "unknown",
		Hostname:              "k2device.kolide.com",
		LogFilePath:           "/var/kolide-k2/k2device-preprod.kolide.com/debug.json",
		LauncherFlagsFilePath: "/etc/kolide-k2/launcher.flags",
	}

	for _, opt := range opts {
		opt(template)
	}

	return template
}

func (td *templateData) parse(text string) (string, error) {
	if td == nil {
		return "", fmt.Errorf("templateData is nil")
	}

	t, err := template.New("menu_template").Parse(text)
	if err != nil {
		return "", fmt.Errorf("could not parse menu template: %w", err)
	}

	var b strings.Builder
	err = t.ExecuteTemplate(&b, "menu_template", td)
	if err != nil {
		return "", fmt.Errorf("could not write menu template output: %w", err)
	}

	return b.String(), nil
}
