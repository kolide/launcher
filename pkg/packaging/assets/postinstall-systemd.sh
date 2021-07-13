#!/bin/sh

if [ ! -z "{{.InfoFilename}}" ]; then
    cat <<EOF > "{{.InfoFilename}}"
{{.InfoJson}}
EOF
fi

set -e

systemctl daemon-reload

systemctl enable launcher.{{.Identifier}}
systemctl restart launcher.{{.Identifier}}
