#!/bin/bash

IDENTIFIER=launcher

function usage() {
    echo "Clean launcher instances from your host"
    echo ""
    echo "sudo $0 [args]"
    echo ""
    echo "  Arguments:"
    echo "    -h --help     Print help text"
    echo "    --remove-db   Remove database files"
    echo "    --identifier  Identifier used in packaging"
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

  countLaunchersQuery="select count(name) as launchers from launchd where name = 'com.${IDENTIFIER}.launcher.plist';"
  launchersInstalled=`osqueryi "${countLaunchersQuery}" --line | awk '{print $3}'`

  if [[ $launchersInstalled -ne 1 ]]; then
    echo "Found $launchersInstalled running launcher instance, but was expecting 1"
    echo "Exiting..."
    exit 1
  fi

  /bin/launchctl stop /Library/LaunchDaemons/com.${IDENTIFIER}.launcher.plist
  /bin/launchctl unload /Library/LaunchDaemons/com.${IDENTIFIER}.launcher.plist
  sleep 3

  rm -rf /etc/${IDENTIFIER}
  rm -f /Library/LaunchDaemons/com.${IDENTIFIER}.launcher.plist
  rm -rf /usr/local/${IDENTIFIER}
  rm -rf /var/log/${IDENTIFIER}

  if [[ $REMOVE_DATABASE ]]; then
    rm -rf /var/${IDENTIFIER}
  fi

  echo "One launcher instance cleaned"
}

while (( "$#" ));
do
case $1 in
    -h | --help)
        usage
        exit 0
        ;;

    --remove-db)
        REMOVE_DATABASE=1
        ;;

    --identifier)
        shift
        IDENTIFIER=$1
        ;;

    *)
        usage
        exit 0
    ;;
esac

shift

done

main
