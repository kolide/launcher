#!/bin/bash

/bin/launchctl stop /Library/LaunchDaemons/com.kolide.launcher.plist
/bin/launchctl unload /Library/LaunchDaemons/com.kolide.launcher.plist
sleep 3
rm -rf /etc/kolide
rm -f /Library/LaunchDaemons/com.kolide.launcher.plist
rm -rf /usr/local/kolide
rm -rf /var/kolide
rm -rf /var/log/kolide
