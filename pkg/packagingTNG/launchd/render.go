package launchd

import (
	"io"
	"text/template"

	"github.com/pkg/errors"
)

type LaunchdOptions struct {
	ThrottleInterval  int
	StandardErrorPath string
	StandardOutPath   string
	Environment       map[string]string
	Flags             []string
}

type Option func(*LaunchdOptions)

func WithEnv(e map[string]string) Option {
	return func(o *LaunchdOptions) {
		o.Environment = e
	}
}

func WithFlags(f []string) Option {
	return func(o *LaunchdOptions) {
		o.Flags = f
	}
}

// renderLaunchDaemon renders a LaunchDaemon to start and schedule the launcher.
func Render(w io.Writer, name string, opts ...Option) error {
	// Create options, and define defaults
	launchdOptions := &LaunchdOptions{
		ThrottleInterval:  60,
		StandardErrorPath: "/tmp/stderr.log", // TODO
		StandardOutPath:   "/tmp/stdout.log", // TODO
	}

	for _, opt := range opts {
		opt(launchdOptions)
	}

	// This could be replaced with an XML library. Work for another day...
	launchDaemonTemplate :=
		`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
    <dict>
        <key>Label</key>
        <string>{{.Name}}</string>
        <key>EnvironmentVariables</key>
        <dict>
{{- if .Opts.Environment}}{{- range $key, $value := .Opts.Environment }}
            <key>$key</key>
            <string>$value</string>
{{- end }}{{- end }}
        </dict>
        <key>ProgramArguments</key>
        <array>
{{- if .Opts.Flags }}{{- range $i, $flag := .Opts.Flags }}
            <string>{{$flag}}</string>
{{- end }}{{- end }}
        </array>
       <key>KeepAlive</key>
        <dict>
            <key>PathState</key>
            <dict>
                <key>TODO</key>
                <true/>
            </dict>
        </dict>
        <key>ThrottleInterval</key>
        <integer>{{.Opts.ThrottleInterval}}</integer>
        <key>StandardErrorPath</key>
        <string>{{.Opts.StandardErrorPath}}</string>
        <key>StandardOutPath</key>
        <string>{{.Opts.StandardOutPath}}</string>
    </dict>
</plist>`

	var data = struct {
		Name string
		Opts LaunchdOptions
	}{
		Name: name,
		Opts: *launchdOptions,
	}

	t, err := template.New("LaunchDaemon").Parse(launchDaemonTemplate)
	if err != nil {
		return errors.Wrap(err, "not able to parse LaunchDaemon template")
	}
	return t.ExecuteTemplate(w, "LaunchDaemon", data)
}
