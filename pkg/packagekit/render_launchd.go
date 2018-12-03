package packagekit

import (
	"io"
	"text/template"

	"github.com/pkg/errors"
)

type launchdOptions struct {
	StandardErrorPath string
	StandardOutPath   string
	ThrottleInterval  int
}

type lOption func(*launchdOptions)

func RenderLaunchd(w io.Writer, initOptions *InitOptions, opts ...lOption) error {
	lOpts := &launchdOptions{
		ThrottleInterval:  60,
		StandardErrorPath: "/tmp/stderr.log", // TODO
		StandardOutPath:   "/tmp/stdout.log", // TODO
	}

	for _, opt := range opts {
		opt(lOpts)
	}

	// This could be replaced with an XML library. Work for another day...
	launchDaemonTemplate :=
		`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
    <dict>
        <key>Label</key>
        <string>{{.Common.Name}}</string>
        <key>EnvironmentVariables</key>
        <dict>
{{- range $key, $value := .Common.Environment }}
            <key>$key</key>
            <string>$value</string>
{{- end }}
        </dict>
        <key>ProgramArguments</key>
        <array>
{{- range $i, $flag := .Common.Flags }}
            <string>{{$flag}}</string>
{{- end }}
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
		Common InitOptions
		Opts   launchdOptions
	}{
		Common: *initOptions,
		Opts:   *lOpts,
	}

	t, err := template.New("LaunchDaemon").Parse(launchDaemonTemplate)
	if err != nil {
		return errors.Wrap(err, "not able to parse LaunchDaemon template")
	}
	return t.ExecuteTemplate(w, "LaunchDaemon", data)
}
