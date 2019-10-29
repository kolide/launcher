#!/bin/sh

[[ $3 != "/" ]] && exit 0

/bin/launchctl stop com.{{.Identifier}}.launcher

if [ ! -z "{{.InfoFilename}}" ]; then
    PACKAGE_FILENAME=$(basename "$PACKAGE_PATH")
    cat <<EOF > "{{.InfoFilename}}"
{{.InfoJson}}
EOF

    plutil -convert xml1 -o  "{{StringsTrimSuffix .InfoFilename `.json`}}.plist" "{{.InfoFilename}}"
fi

# Sleep to let the stop take effect
sleep 5

/bin/launchctl unload {{.Path}}
/bin/launchctl load {{.Path}}
