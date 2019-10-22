#!/bin/sh

[[ $3 != "/" ]] && exit 0

/bin/launchctl stop com.{{.Identifier}}.launcher

sleep 5

/bin/launchctl unload {{.Path}}
/bin/launchctl load {{.Path}}
