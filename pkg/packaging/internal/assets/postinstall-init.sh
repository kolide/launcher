#!/bin/sh

if [ ! -z "{{.InfoFilename}}" ]; then
    cat <<EOF > "{{.InfoFilename}}"
{{.InfoJson}}
EOF
fi

if [ "$1" = "configure" ] || [ "$1" = "abort-upgrade" ]; then
    if [ -e "/etc/init.d/{{.Identifier}}-launcher" ]; then
        chmod 755 "/etc/init.d/{{.Identifier}}-launcher"
        update-rc.d "{{.Identifier}}-launcher" defaults >/dev/null
        invoke-rc.d "{{.Identifier}}-launcher" start || exit $?
    fi
fi
