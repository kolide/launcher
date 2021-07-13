#!/bin/sh

# upstart's stop and restart commands error out if the daemon isn't
# running. So stop and start are separate, and `set -e` is after the
# stop.

if [ ! -z "{{.InfoFilename}}" ]; then
    cat <<EOF > "{{.InfoFilename}}"
{{.InfoJson}}
EOF
fi

stop launcher-{{.Identifier}}
set -e
start launcher-{{.Identifier}}
