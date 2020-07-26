#!upstart
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
{{- end }}
