#!/bin/bash

# Make sure we're in the right directory -- want to be at top-level launcher, not in docs directory
if [[ "$0" != "./docs/multipass/multipass.sh" ]]
then
    echo "Must run script from top level of launcher"
    exit 1
fi

# Check deps: validate VM name
vmname=$1
if [ -z "$vmname" ]
then
    echo "Usage: ./docs/multipass/multipass.sh <vmname> [gnome|xfce|cinnamon|mate|kde]"
    exit 1
fi

foundvm=$({ multipass info $vmname || true; }) &>/dev/null
if [[ $foundvm == *"State"* ]]
then
    echo "VM already exists with name $vmname -- choose a new name or run: multipass delete $vmname && multipass purge"
    exit 1
fi

# Check deps: get desktop environment
desktopenv="gnome"
if [ ! -z "$2" ]
then
    if [[ "$2" != "gnome" && "$2" != "xfce" && "$2" != "cinnamon" && "$2" != "mate" && "$2" != "kde" ]]
    then
        echo "Desktop environment must be one of: gnome, xfce, cinnamon, mate, kde"
        exit 1
    fi
    desktopenv=$2
fi

# Check deps: make sure multipass is installed
if ! command -v multipass &> /dev/null
then
    echo "Multipass not installed; please install it: https://multipass.run/install"
    exit 1
fi

# Check deps: make sure jq is installed
if ! command -v jq &> /dev/null
then
    echo "Please install jq"
    exit 1
fi

# Check deps: warn if firewall is enabled
firewallstate=$(/usr/libexec/ApplicationFirewall/socketfilterfw --getglobalstate)
if [[ $firewallstate == *"enabled"* ]]
then
    echo "Firewall is enabled -- will attempt to proceed; if script times out, please disable firewall, restart your computer, and try again"
fi

# Create the VM
echo "Creating VM..."
multipass launch 22.04 -n $vmname -c 2 -m 4G -d 50G

# Move dependencies to the VM: launcher
echo "Building launcher..."
make deps
go run cmd/make/make.go -targets=launcher -linkstamp --os linux --arch arm64
multipass transfer ./build/linux.arm64/launcher $vmname:.

# Prepare to install deps inside VM
echo "Installing dependencies in VM..."
multipass exec $vmname -- sudo apt-get -qq update

# Install xrdp in the VM
echo "Installing xrdp..."
multipass exec $vmname -- sudo apt-get -y install xrdp

# Install a desktop environment
if [ "$desktopenv" = "gnome" ]
then
    echo "Installing GNOME..."
    multipass exec $vmname -- sudo apt-get -y install ubuntu-desktop

    echo "Enabling user extensions for GNOME and restarting VM"
    multipass exec $vmname -- gsettings set org.gnome.shell disable-user-extensions false
    multipass exec $vmname -- gnome-extensions enable ubuntu-appindicators@ubuntu.com
    multipass restart $vmname

    # Confirm that it's back online before proceeding
    vmstate=$(multipass info $vmname --format json | jq --arg VMNAME "$vmname" '.info | to_entries | .[] | select(.key==$VMNAME) | .value.state')
    while [ "$vmstate" != "\"Running\"" ]
    do
        vmstate=$(multipass info $vmname --format json | jq --arg VMNAME "$vmname" '.info | to_entries | .[] | select(.key==$VMNAME) | .value.state')
        echo "Current state: $vmstate"
        sleep 5
    done
elif [ "$desktopenv" = "xfce" ]
then
    echo "Installing Xfce..."
    multipass exec $vmname -- sudo apt-get -y install xfce4 xfce4-terminal
elif [ "$desktopenv" = "cinnamon" ]
then
    echo "Installing Cinnamon..."
    multipass exec $vmname -- sudo apt-get -y install cinnamon
elif [ "$desktopenv" = "mate" ]
then
    echo "Installing MATE..."
    multipass exec $vmname -- sudo apt-get -y install ubuntu-mate-desktop
elif [ "$desktopenv" = "kde" ]
then
    echo "Installing KDE..."
    multipass exec $vmname -- sudo apt-get -y install kde-plasma-desktop
fi

# Install a browser (needed for launcher notification actions)
echo "Installing Firefox..."
multipass exec $vmname -- sudo apt-get -y install firefox

# Install and start mockoon
echo "Installing and starting mockoon..."
multipass transfer ./docs/multipass/mockoon-control-server.json $vmname:.
multipass exec $vmname -- sudo apt-get -y install nodejs npm
multipass exec $vmname -- sudo npm install -g n
multipass exec $vmname -- sudo n lts
multipass exec $vmname -- sudo npm install -g @mockoon/cli
multipass exec $vmname -- mockoon-cli start --data mockoon-control-server.json

# Prepare launcher
echo "Preparing launcher..."
multipass exec $vmname -- chmod +x launcher
multipass exec $vmname -- sudo mkdir -p /var/kolide-k2/localhost

# Install osquery and copy it to a known location
echo "Installing osquery..."
multipass exec $vmname -- wget -L "https://github.com/osquery/osquery/releases/download/5.7.0/osquery_5.7.0-1.linux_arm64.deb"
multipass exec $vmname -- sudo apt-get -y install ./osquery_5.7.0-1.linux_arm64.deb
multipass exec $vmname -- sudo mkdir -p /usr/local/kolide-k2/bin
multipass exec $vmname -- sudo cp /opt/osquery/bin/osqueryd /usr/local/kolide-k2/bin/

# Set password
yes ubuntu | multipass exec $vmname -- sudo passwd ubuntu

# Get IP for Microsoft Remote Desktop
vmip=$(multipass info $vmname --format json | jq --arg VMNAME "$vmname" '.info | to_entries | .[] | select(.key==$VMNAME) | .value.ipv4 | .[]')

# Print instructions
echo "Add PC via Microsoft Remote Desktop, putting $vmip for the PC name and configuring ubuntu/ubuntu for the user account"
echo "Install Microsoft Remote Desktop from here if you don't have it already: https://apps.apple.com/us/app/microsoft-remote-desktop/id1295203466?mt=12"
echo "Once logged in, open your terminal and run launcher:"
echo "sudo LAUNCHER_SKIP_UPDATES=true ./launcher --root_directory /var/kolide-k2/localhost --hostname localhost:3000 --transport jsonrpc --enroll_secret secret --debug"

exit 0
