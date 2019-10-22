#!/bin/sh

[[ $3 != "/" ]] && exit 0

/bin/launchctl stop com.{{.Identifier}}.launcher

if [ ! -z "{{.InfoOutput}}" ]; then
    cat <<EOF > "{{.InfoOutput}}"
{{.InfoJson}}
EOF
fi

# Sleep to let the stop take effect
sleep 5

/bin/launchctl unload {{.Path}}
/bin/launchctl load {{.Path}}
