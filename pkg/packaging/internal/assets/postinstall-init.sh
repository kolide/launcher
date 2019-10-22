#!/bin/sh

if [ ! -z "{{.InfoOutput}}" ]; then
    cat <<EOF > "{{.InfoOutput}}"
{{.InfoJson}}
 EOF
fi

sudo service launcher.{{.Identifier}} restart
