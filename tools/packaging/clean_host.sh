#!/bin/bash

function usage() {
    echo "Clean launcher instances from your host"
    echo ""
    echo "sudo $0 [args]"
    echo ""
    echo "  Arguments:"
    echo "    -h --help     Print help text"
    echo "    --remove-db   Remove database files"
    echo ""
}

function ensureMacos() {
  if [[ `uname` != 'Darwin' ]]; then
    echo "This script must be run on macOS:"
    echo "  sudo $0 --help"
    exit 1
  fi
}

function ensureRoot() {
  if [[ $EUID != 0 ]]; then
     echo "This script must be run as root:"
     echo "  sudo $0 --help"
     exit 1
  fi
}

function main() {
  ensureMacos
  ensureRoot

  countLaunchersQuery="select count(name) as launchers from launchd where name = 'com.kolide.launcher.plist';"
  launchersInstalled=`osqueryi "${countLaunchersQuery}" --line | awk '{print $3}'`

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
  rm -rf /var/log/kolide

  if [[ $REMOVE_DATABASE ]]; then
    rm -rf /var/kolide
  fi

  echo "One launcher instance cleaned"
}

for i in "$@"
do
case $i in
    -h | --help)
    usage
    exit 0
    ;;
    --remove-db)
    REMOVE_DATABASE=1
    shift
    ;;
    *)
    ;;
esac
done

main
