# Testing Linux on macOS using Multipass

## Install

Install Multipass following instructions [here](https://multipass.run/install).

Install [Microsoft Remote Desktop](https://apps.apple.com/us/app/microsoft-remote-desktop/id1295203466?mt=12).

If using Mockoon, [install Mockoon](https://mockoon.com/download/).

## Caveats

You will need to comment out the empty seat check in `ee/consoleuser/consoleuser_linux.go`.

If you want to test notifications opening browser windows, you will need to set the `DISPLAY`
environment variable in `ee/desktop/notify/listener_linux.go` when running the command to
open the browser.

If you're using GNOME, after setting up your VM you will need to run the following and then reboot
in order to see the systray item:

```
gsettings set org.gnome.shell disable-user-extensions false
gnome-extensions enable ubuntu-appindicators@ubuntu.com
```

## Instructions

Create the VM:

```
vmname="gnome-sweet-gnome" # set to whatever you want
multipass launch 22.04 -n $vmname -c 2 -m 4G -d 50G
```

Build launcher and transfer it to the VM:

```
go run cmd/make/make.go -targets=launcher -linkstamp --os linux --arch arm64
multipass transfer ./build/linux.arm64/launcher $vmname:.
```

Download an osquery executable with the correct architecture from [here](https://github.com/osquery/osquery/releases),
then transfer it to the VM:

```
multipass transfer ~/Downloads/osquery_5.6.0-1.linux_arm64.deb $vmname:.
```

Get shell access to the VM:

```
multipass shell $vmname
```

Install dependencies:

```
# Prepare to install dependencies
sudo apt update

# Install xrdp
sudo apt install -y xrdp

# Install a desktop environment -- pick one:
# GNOME: sudo apt install -y ubuntu-desktop
# cinnamon: sudo apt install -y cinnamon
# MATE: sudo apt install -y ubuntu-mate-desktop
# xfce: sudo apt install -y xfce4 xfce4-terminal
# kde: sudo apt install -y kde-plasma-desktop

# Prepare launcher and directories it depends on
chmod +x launcher
sudo mkdir -p /var/kolide-k2/<some k2 server name here>

# Install osquery and copy it to a known location
sudo apt install ./osquery_5.6.0-1.linux_arm64.deb
sudo mkdir -p /usr/local/kolide-k2/bin
sudo cp /opt/osquery/bin/osqueryd /usr/local/kolide-k2/bin/

# If you're testing notification actions, you'll want a browser:
# sudo apt install -y firefox

# Set the user password for login later
sudo passwd ubuntu

# Done
exit
```

Find the IP address for your VM:

```
multipass list | grep $vmname
```

Add the VM via Microsoft Remote Desktop:
1. Click add PC
1. PC name: put IP address you found above
1. User account: Can do "Ask when required" or configure "ubuntu/<password you set earlier>" now
1. Friendly name: helpful if you've got a couple different ones

Access the connection you just added and log in to the VM.

Open a terminal, and run the following to start launcher:

```
sudo LAUNCHER_SKIP_UPDATES=true ./launcher --root_directory <same directory you created earlier> \
    --hostname localhost:3000 --transport jsonrpc --enroll_secret secret --debug
```

## Using Mockoon for the control server

Transfer the Mockoon JSON file to your VM:

```
multipass transfer ./docs/mockoon-control-server.json $vmname:.
```

In the VM, install the dependencies:

```
sudo apt install nodejs npm -y
sudo npm install -g n
sudo n lts
sudo npm install -g @mockoon/cli
```

Then, in the VM, run Mockoon:

```
mockoon-cli start --data mockoon-control-server.json
```
