#!/bin/sh

if [ ! -z "{{.InfoFilename}}" ]; then
    cat <<EOF > "{{.InfoFilename}}"
{{.InfoJson}}
 EOF
fi

sudo service launcher.{{.Identifier}} restart
