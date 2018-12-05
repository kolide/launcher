package packagekit

import (
	"html/template"
	"io"
	"strings"

	"github.com/pkg/errors"
)

type systemdOptions struct {
	Restart    string
	RestartSec int
}

//type sOption func(*systemdOptions)

func RenderSystemd(w io.Writer, initOptions *InitOptions) error {
	sOpts := &systemdOptions{
		Restart:    "on-failure",
		RestartSec: 3,
	}

	// Prepend a "" so that the merged output looks a bit cleaner in the systemd file
	if len(initOptions.Flags) > 0 {
		initOptions.Flags = append([]string{""}, initOptions.Flags...)
	}

	systemdTemplate :=
		`[Unit]
Description={{.Common.Description}}
After=network.service syslog.service

[Service]
{{- if .Common.Environment}}{{- range $key, $value := .Common.Environment }}
Environment=$key=$value
{{- end }}{{- end }}
ExecStart={{.Common.Path}}{{ StringsJoin .Common.Flags " \\\n" }}
Restart={{.Opts.Restart}}
RestartSec={{.Opts.RestartSec}}

[Install]
WantedBy=multi-user.target`

	var data = struct {
		Common InitOptions
		Opts   systemdOptions
	}{
		Common: *initOptions,
		Opts:   *sOpts,
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
