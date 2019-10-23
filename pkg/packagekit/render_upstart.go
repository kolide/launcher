package packagekit

import (
	"context"
	"html/template"
	"io"
	"strings"

	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

// upstartOptions contains upstart specific options that are passed to
// the rendering template.
type upstartOptions struct {
	PreStartScript  []string
	PostStartScript []string
	PreStopScript   []string
	Expect          string
}

type UpstartOption func(*upstartOptions)

// WithExpect sets the expect option. This is how upstart tracks the
// pid of the daemon. See http://upstart.ubuntu.com/cookbook/#expect
func WithExpect(s string) UpstartOption {
	return func(uo *upstartOptions) {
		uo.Expect = s
	}
}

func WithPreStartScript(s []string) UpstartOption {
	return func(uo *upstartOptions) {
		uo.PreStartScript = s
	}
}

func WithPostStartScript(s []string) UpstartOption {
	return func(uo *upstartOptions) {
		uo.PostStartScript = s
	}
}

func WithPreStopScript(s []string) UpstartOption {
	return func(uo *upstartOptions) {
		uo.PreStopScript = s
	}
}

func RenderUpstart(ctx context.Context, w io.Writer, initOptions *InitOptions, uOpts ...UpstartOption) error {
	_, span := trace.StartSpan(ctx, "packagekit.Upstart")
	defer span.End()

	uOptions := &upstartOptions{}
	for _, uOpt := range uOpts {
		uOpt(uOptions)
	}

	// Prepend a "" so that the merged output looks a bit cleaner in the rendered templates
	if len(initOptions.Flags) > 0 {
		initOptions.Flags = append([]string{""}, initOptions.Flags...)
	}

	upstartTemplate := `#!upstart
#
# Name: {{ .Common.Name }}
# Description: {{.Common.Description}}

{{ if .Opts.Expect }}
expect {{ .Opts.Expect }}
{{- end }}

# Start and stop on boot events
start on net-device-up
stop on shutdown

# Respawn upto 15 times within 5 seconds.
# Exceeding that will be considered a failure
respawn
respawn limit 15 5

# Send logs to the default upstart location, /var/log/upstart/
# (This should be rotated by the upstart managed logrotate)
console log

# Environment Variables
{{- if .Common.Environment}}{{- range $key, $value := .Common.Environment }}
env {{$key}}={{$value}}
{{- end }}{{- end }}

exec {{.Common.Path}}{{ StringsJoin .Common.Flags " \\\n  " }}

{{- if .Opts.PreStopScript }}
pre-stop script
{{StringsJoin .Opts.PreStopScript "\n"}}
end script
{{- end }}

{{- if .Opts.PreStartScript }}
pre-start script
{{StringsJoin .Opts.PreStartScript "\n"}}
end script
{{- end }}

{{- if .Opts.PostStartScript }}
post-start script
{{StringsJoin .Opts.PostStartScript "\n"}}
end script
{{- end }}`

	var data = struct {
		Common InitOptions
		Opts   upstartOptions
	}{
		Common: *initOptions,
		Opts:   *uOptions,
	}

	funcsMap := template.FuncMap{
		"StringsJoin": strings.Join,
	}

	t, err := template.New("UpstartConf").Funcs(funcsMap).Parse(upstartTemplate)
	if err != nil {
		return errors.Wrap(err, "not able to parse Upstart template")
	}
	return t.ExecuteTemplate(w, "UpstartConf", data)
}
