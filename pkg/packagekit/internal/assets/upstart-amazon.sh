#!upstart
#
# Name: {{ .Common.Name }}
description     "{{.Common.Description}}"
author          "kolide.com"

start on (runlevel [345] and started network)
stop on (runlevel [!345] or stopping network)

# Respawn upto 15 times within 5 seconds.
# Exceeding that will be considered a failure
respawn
respawn limit 15 5

# Environment Variables
{{- if .Common.Environment}}{{- range $key, $value := .Common.Environment }}
env {{$key}}={{$value}}
{{- end }}{{- end }}

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
