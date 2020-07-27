#!upstart
#
# Name: {{ .Common.Name }}
description     "{{.Common.Description}} for {{.Common.Identifier}}"
author          "kolide.com"

{{ if .Opts.Expect }}
expect {{ .Opts.Expect }}
{{- end }}

{{ if .Opts.StartOn }}
start on {{ .Opts.StartOn }}
{{- end }}
{{ if .Opts.StopOn }}
stop on {{ .Opts.StopOn }}
{{- end }}

# Respawn upto 15 times within 5 seconds.
# Exceeding that will be considered a failure
respawn
respawn limit 15 5

{{ if .Opts.ConsoleLog }}
# Send logs to the default upstart location, /var/log/upstart/
# (This should be rotated by the upstart managed logrotate)
console log
{{- end }}

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
