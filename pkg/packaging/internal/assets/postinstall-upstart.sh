#!/bin/sh

# upstart's stop and restart commands error out if the daemon isn't
# running. So stop and start are separate, and `set -e` is after the
# stop.

if [ ! -z "{{.InfoOutput}}" ]; then
    cat <<EOF > "{{.InfoOutput}}"
{{.InfoJson}}
EOF
fi

stop launcher-{{.Identifier}}
set -e
start launcher-{{.Identifier}}
