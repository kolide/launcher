#!/bin/bash

if [[ $EUID -ne 0 ]]; then
   echo "This script must be run as root:"
   echo "  sudo $0"
   exit 1
fi

countLaunchersQuery="select count(name) as launchers from launchd where name = 'com.kolide.launcher.plist';"
launchersInstalled=`osqueryi "${countLaunchersQuery}" --line | awk {'print $3'}`

if [[ $launchersInstalled -ne 1 ]]; then
  echo "Found $launchersInstalled running launcher instance, but was expecting 1"
  echo "Exiting..."
  exit 1
fi

/bin/launchctl stop /Library/LaunchDaemons/com.kolide.launcher.plist
/bin/launchctl unload /Library/LaunchDaemons/com.kolide.launcher.plist
sleep 3
rm -rf /etc/kolide
rm -f /Library/LaunchDaemons/com.kolide.launcher.plist
rm -rf /usr/local/kolide
rm -rf /var/kolide
rm -rf /var/log/kolide

echo "One launcher instance cleaned"
