# Testing Launcher on Ubuntu using Multipass

## Install

Install Multipass following instructions [here](https://multipass.run/install).

Install [Microsoft Remote Desktop](https://apps.apple.com/us/app/microsoft-remote-desktop/id1295203466?mt=12).

The script also uses `jq` to parse JSON output, which can be installed with `brew install jq`.

You may want to install [Mockoon](https://mockoon.com/download/) locally to more easily update
the JSON configuration file.

## Code changes required for testing various aspects of the UI

You will need to comment out the empty seat check in `ee/consoleuser/consoleuser_linux.go` --
otherwise launcher will not start a desktop process for your user.

If you want to test notifications opening browser windows, you will need to set the `DISPLAY`
environment variable in `ee/desktop/notify/listener_linux.go` when running the command to
open the browser. (In the VM, run `echo $DISPLAY` to find the correct value.)

## Usage

Use the provided script `multipass.sh` to start a Multipass VM, specifying the VM name
and the desired desktop environment (one of `gnome`, `xfce`, `cinnamon`, `mate`, or `kde`,
defaulting to `gnome` if not specified):

```
./docs/multipass/multipass.sh my-new-vm kde
```

Add the VM via Microsoft Remote Desktop:

1. Click add PC
1. PC name: put IP address given in the output of the script (or find it from `multipass list`)
1. User account: Can do "Ask when required" or configure "ubuntu/ubuntu" now
1. Friendly name: helpful if you've got a couple different ones

Access the VM via Microsoft Remote Desktop, open a terminal window, and start launcher:

```
sudo LAUNCHER_SKIP_UPDATES=true ./launcher --root_directory /var/kolide-k2/localhost \
    --hostname localhost:3000 --transport jsonrpc --enroll_secret secret --debug
```

## Caveats and troubleshooting

### Firewall

If commands to multipass are timing out, you may need to disable your firewall and
log out/log back in again, and/or restart multipassd:

```
sudo launchctl unload /Library/LaunchDaemons/com.canonical.multipassd.plist
sudo launchctl load /Library/LaunchDaemons/com.canonical.multipassd.plist
```

### Rebooting VMs

If you restart your VM, start up the Mockoon server again:

```
mockoon-cli start --data mockoon-control-server.json
```

### VM stuck in "Unknown" state

VMs usually seem to get stuck in the "Unknown" state due to firewall issues.
Try the troubleshooting given in that section and/or recreate the VM.

### I want an image besides Ubuntu

It looks like this may be supported in the future by Multipass -- see [this issue](https://github.com/canonical/multipass/issues/1260).
