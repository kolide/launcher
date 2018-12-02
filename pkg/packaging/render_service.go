package packaging

import (
	"fmt"
	"io"
	"text/template"

	"github.com/pkg/errors"
)

type ServiceOptions struct {
	ServerHostname    string
	RootDirectory     string
	LauncherPath      string
	OsquerydPath      string
	LogDirectory      string
	SecretPath        string
	ServiceName       string
	InsecureGrpc      bool
	Insecure          bool
	Autoupdate        bool
	UpdateChannel     string
	Control           bool
	InitialRunner     bool
	ControlHostname   string
	DisableControlTLS bool
	CertPins          string
	RootPEM           string
}

type Flavor string

const (
	LaunchD Flavor = "launchd"
	SystemD        = "systemd"
	Init           = "init"
)

func (o *ServiceOptions) Render(w io.Writer, flavor Flavor) error {
	switch flavor {
	case LaunchD:
		return o.launchd(w)
	case SystemD:
		return o.systemd(w)
	case Init:
		return o.init(w)
	default:
		return fmt.Errorf("unknown flavor %s", flavor)
	}
}

func (o *ServiceOptions) launchd(w io.Writer) error {
	launchDaemonTemplate :=
		`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
    <dict>
        <key>Label</key>
        <string>{{.ServiceName}}</string>
        <key>EnvironmentVariables</key>
        <dict>
            <key>KOLIDE_LAUNCHER_ROOT_DIRECTORY</key>
            <string>{{.RootDirectory}}</string>
            <key>KOLIDE_LAUNCHER_HOSTNAME</key>
            <string>{{.ServerHostname}}</string>
            <key>KOLIDE_LAUNCHER_ENROLL_SECRET_PATH</key>
            <string>{{.SecretPath}}</string>
            <key>KOLIDE_LAUNCHER_OSQUERYD_PATH</key>
            <string>{{.OsquerydPath}}</string>{{if .Control}}
            <key>KOLIDE_CONTROL_HOSTNAME</key>
            <string>{{.ControlHostname}}</string>{{end}}{{if .Autoupdate}}
            <key>KOLIDE_LAUNCHER_UPDATE_CHANNEL</key>
            <string>{{.UpdateChannel}}</string>{{end}}{{if .CertPins }}
            <key>KOLIDE_LAUNCHER_CERT_PINS</key>
            <string>{{.CertPins}}</string>{{end}}{{if .RootPEM }}
            <key>KOLIDE_LAUNCHER_ROOT_PEM</key>
            <string>{{.RootPEM}}</string>{{end}}
        </dict>
        <key>KeepAlive</key>
        <dict>
            <key>PathState</key>
            <dict>
                <key>{{.SecretPath}}</key>
                <true/>
            </dict>
        </dict>
        <key>ThrottleInterval</key>
        <integer>60</integer>
        <key>ProgramArguments</key>
        <array>
            <string>{{.LauncherPath}}</string>
            {{if .InsecureGrpc}}
            <string>--insecure_grpc</string>
			{{end}}
			{{if .Insecure}}
            <string>--insecure</string>{{end}}
			{{if .Autoupdate}}
            <string>--autoupdate</string>
			{{end}}
			{{if .Control}}
            <string>--control</string>
			{{end}}
			{{if .InitialRunner}}
            <string>--with_initial_runner</string>
			{{end}}
			{{if .DisableControlTLS}}
            <string>--disable_control_tls</string>
			{{end}}
        </array>
        <key>StandardErrorPath</key>
        <string>{{.LogDirectory}}/launcher-stderr.log</string>
        <key>StandardOutPath</key>
        <string>{{.LogDirectory}}/launcher-stdout.log</string>
    </dict>
</plist>`
	t, err := template.New("LaunchDaemon").Parse(launchDaemonTemplate)
	if err != nil {
		return errors.Wrap(err, "not able to parse LaunchDaemon template")
	}
	return t.ExecuteTemplate(w, "LaunchDaemon", o)
}

func (o *ServiceOptions) systemd(w io.Writer) error {
	// renderSystemdService renders a systemd service to start and schedule the launcher.
	systemdTemplate :=
		`[Unit]
Description=The Kolide Launcher
After=network.service syslog.service

[Service]
ExecStart={{.LauncherPath}} \
--root_directory={{.RootDirectory}} \
--hostname={{.ServerHostname}} \
--enroll_secret_path={{.SecretPath}} \{{if .InsecureGrpc}}
--insecure_grpc \{{end}}{{if .Insecure}}
--insecure \{{end}}{{if .Control}}
--control \
--control_hostname={{.ControlHostname}} \{{end}}{{if .DisableControlTLS}}
--disable_control_tls \{{end}}{{if .InitialRunner}}
--with_initial_runner \{{end}}{{if .Autoupdate}}
--autoupdate \
--update_channel={{.UpdateChannel}} \{{end}}{{if .CertPins }}
--cert_pins={{.CertPins}} \{{end}}{{if .RootPEM}}
--root_pem={{.RootPEM}} \{{end}}
--osqueryd_path={{.OsquerydPath}}
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target`
	t, err := template.New("systemd").Parse(systemdTemplate)
	if err != nil {
		return errors.Wrap(err, "not able to parse systemd template")
	}
	return t.ExecuteTemplate(w, "systemd", o)
}

func (o *ServiceOptions) init(w io.Writer) error {
	initdTemplate := `#!/bin/sh
set -e
NAME="{{.ServiceName}}"
DAEMON="{{.LauncherPath}}"
DAEMON_OPTS="--root_directory={{.RootDirectory}} \
--hostname={{.ServerHostname}} \
--enroll_secret_path={{.SecretPath}} \{{if .InsecureGrpc}}
--insecure_grpc \{{end}}{{if .Insecure}}
--insecure \{{end}}{{if .InitialRunner}}
--with_initial_runner \{{end}}{{if .Autoupdate}}
--autoupdate \
--update_channel={{.UpdateChannel}} \{{end}}{{if .CertPins}}
--cert_pins={{.CertPins}} \{{end}}{{if .RootPEM}}
--root_pem={{.RootPEM}} \{{end}}
--osqueryd_path={{.OsquerydPath}}"

export PATH="${PATH:+$PATH:}/usr/sbin:/sbin"

is_running() {
    start-stop-daemon --status --exec $DAEMON
}
case "$1" in
  start)
        echo "Starting daemon: "$NAME
        start-stop-daemon --start --quiet --background --exec $DAEMON -- $DAEMON_OPTS
        ;;
  stop)
        echo "Stopping daemon: "$NAME
        start-stop-daemon --stop --quiet --oknodo --exec $DAEMON
        ;;
  restart)
        echo "Restarting daemon: "$NAME
        start-stop-daemon --stop --quiet --oknodo --retry 30 --exec $DAEMON
        start-stop-daemon --start --quiet --background --exec $DAEMON -- $DAEMON_OPTS
        ;;
  status)
    if is_running; then
        echo "Running"
    else
        echo "Stopped"
        exit 1
    fi
    ;;
  *)
        echo "Usage: "$1" {start|stop|restart|status}"
        exit 1
esac

exit 0
`

	t, err := template.New("initd").Parse(initdTemplate)
	if err != nil {
		return errors.Wrap(err, "not able to parse initd template")
	}
	return t.ExecuteTemplate(w, "initd", o)
}
