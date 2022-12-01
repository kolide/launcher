#!/bin/bash

# This script installs osquery for Debian-based dev containers if it is not
# already present.
#
# Usage notes:
# Update your devcontainer.json's "postCreateCommand" to call this script:
# "postCreateCommand": "sudo ./tools/vscode-debugging/postCreateCommand.sh"

# Install osquery if not present
if ! command -v osqueryd &> /dev/null
then
    echo "installing osquery:"

     # Prepare to install osquery -- we need software-properties-common for add-apt-repository
    apt-get update
    apt-get install -y software-properties-common
    apt-get update
    export OSQUERY_KEY=1484120AC4E9F8A1A577AEEE97A80C63C9D8B80B
    apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys $OSQUERY_KEY
    add-apt-repository 'deb [arch=amd64] https://pkg.osquery.io/deb deb main'
    apt-get update

    # Do the install
    apt-get install -y osquery
fi

echo "done"
