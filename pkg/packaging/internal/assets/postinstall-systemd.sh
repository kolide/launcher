#!/bin/sh

if [ ! -z "{{.InfoOutput}}" ]; then
    cat <<EOF > "{{.InfoOutput}}"
{{.InfoJson}}
EOF
fi

set -e

systemctl daemon-reload

systemctl enable launcher.{{.Identifier}}
systemctl restart launcher.{{.Identifier}}
