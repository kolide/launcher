#!/bin/sh

# As of the Big Sur general release, Apple M1 machines no longer ship
# Rosetta 2. Instead, this must be installed manually. As osquery does
# not yet ship a universal binary, launcher requires rosetta2.
#
# During an interactive install, the user is prompted to okay a
# rosetta2 install, but this does not appear to happen during an MDM
# install. So, we need to trigger than from a preinstall script.

# If we're not Big Sur (build 20x), exit
if [[ "$(/usr/bin/sw_vers -buildVersion)" != 20* ]]; then
    exit 0
fi

# If we're not arm, exit
if [ "$(/usr/bin/arch)" != "arm64" ]; then
    exit 0
fi

# If it's already installed, exit.  If this check misfires, we'll
# invoke software update an extra time, which should be okay.
if [[ -f "/Library/Apple/System/Library/LaunchDaemons/com.apple.oahd.plist" ]]; then
    exit 0
fi

# report errors
set -e

/usr/sbin/softwareupdate --install-rosetta --agree-to-license
