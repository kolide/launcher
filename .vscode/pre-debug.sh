#!/bin/bash

set -e

osquerydPath=$(which osqueryd)
mkdir -p /tmp/launcher-root-dev/bin
yes | cp $osquerydPath /tmp/launcher-root-dev/bin/osqueryd

make deps
make build

yes | cp build/osquery-extension.ext cmd/launcher/osquery-extension.ext

