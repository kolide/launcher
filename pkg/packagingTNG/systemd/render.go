package systemd

import (
	"html/template"
	"io"
	"strings"

	"github.com/pkg/errors"
)

type SystemdOptions struct {
	Description  string
	LauncherPath string
	Environment  map[string]string
	Flags        []string
	Restart      string
	RestartSec   int
}

type Option func(*SystemdOptions)

func WithEnv(e map[string]string) Option {
	return func(o *SystemdOptions) {
		o.Environment = e
	}
}

func WithFlags(f []string) Option {
	return func(o *SystemdOptions) {
		o.Flags = f
	}
}

func Render(w io.Writer, name string, opts ...Option) error {
	systemdOptions := &SystemdOptions{
		Description:  "The Kolide Launcher",
		LauncherPath: "TODO",
		Restart:      "on-failure",
		RestartSec:   3,
	}

	for _, opt := range opts {
		opt(systemdOptions)
	}

	if systemdOptions.Description == "" {
		systemdOptions.Description = name
	}

	systemdTemplate :=
		`[Unit]
Description={{.Opts.Description}}
After=network.service syslog.service

[Service]
{{- if .Opts.Environment}}{{- range $key, $value := .Opts.Environment }}
Environment=$key=$value
{{- end }}{{- end }}
ExecStart={{.Opts.LauncherPath}} {{ StringsJoin .Opts.Flags " \\\n" }}
Restart={{.Opts.Restart}}
RestartSec={{.Opts.RestartSec}}

[Install]
WantedBy=multi-user.target`

	var data = struct {
		Name string
		Opts SystemdOptions
	}{
		Name: name,
		Opts: *systemdOptions,
	}

	funcsMap := template.FuncMap{
		"StringsJoin": strings.Join,
	}

	t, err := template.New("SystemdUnit").Funcs(funcsMap).Parse(systemdTemplate)
	if err != nil {
		return errors.Wrap(err, "not able to parse Systemd Unit template")
	}
	return t.ExecuteTemplate(w, "SystemdUnit", data)

}
