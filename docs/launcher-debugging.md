# Debugging Launcher

## Reading Logs
To debug launcher, look at the logs. Depending on what's happening,
you may need to enable debug level logs.

## Logs

Launcher logs to stdout and stderr. Where these are placed, depends on
your system, and how launcher is packaged.

### Package Identifier

When launcher is packaged, it uses an _identifier_ to allow multiple
instances to co-exist. This _identifier_ appears in the systemd unit
name, and the logging paths. It default to `launcher`

For [Kolide
Cloud](https://kolide.com/?utm_source=oss&utm_medium=readme&utm_campaign=launcher),
this identifier has been `kolide`, `launcher` and `kolide-app`.

### MacOS Logs

On MacOS Launcher will generally be running via launchd. Launchd has
an option for where to route logs. The launcher packaging usually sets
this to be in the directory `/var/log/<identifier>/`

### Linux Logs

Most modern linux systems use systemd, and the associated
journald. The unit file is likely named `launcher`, so logs can be
viewed with `journalctl -u launcher`

### Windows Logs

When launcher is running as a service, it logs to the windows event
log system. You should be able to see logs there. 

## Enabling Debug Mode

When running on a posix system, launcher can be toggled into debug
logging mode. You can do this by sending launcher a `USR2` signal.

For example `pkill -USR2 launcher`

Note: windows does not support this as a runtime change

## Running in the foreground

Often, the easiest way to debug launcher is to simply run it in the
foreground.

1. Ensure it's stopped in your init system
2. Look at your init script / systemd unit file / service definition
3. Add a debug option, and run

### Special windows foreground mode

Windows services are a bit different than programs. On windows,
launcher has three modes of running, they all support the `-config`
option.

1. Foreground mode. Invoked as `launcher`, it runs as a windows exe utable
1. Service Mode. Invoked as `launcher svc`, this will only work as a service
1. Service Foreground. Invoked as `launcher svc-fg` this uses golang's
   debug framework to run the service in the foreground. It
   additionally sets the logging to debug mode.

Using `svc-fg` is the recommended approach

## Live Debugging with VS Code

### Prerequisites

1. [Install VS Code](https://code.visualstudio.com/download)
1. [Install VS Code Go Extension](https://code.visualstudio.com/docs/languages/go)
1. Osqueryd is available in your path
1. Open launcher repo with VS Code
* if this is your first time using the VS Code go extension, you'll be prompted to install various go packages when you start debugging

### Debugging over socket

1. Press cmd+p on macOS
1. Type `debug socket`
1. Press enter
1. Lauch osquery with `osqueryd --connect /tmp/osq.sock -S`

Now you should be able to set break points in VS Code and hit them by executing queries.

### Debugging against local K2 server (only available to Kolide employees)

1. Log into your local instance of K2 > Inventory > Add Device, the enroll_secret will be displayed in the launcher command
1. Save your enroll_secret to a file named `k2_enroll_secret` at the root of your launcher repository
1. Either copy the {k2-repo}/tmp/localhost.crt to the root of your launcher repository or create a sym like from {k2-repo}/tmp/localhost.crt localhost.crt at the root of your launcher repository
     ```sh
      # symlink cmd
      ln -s <k2-repo>/tmp/localhost.crt <launcher-repo>/localhost.crt
      ```
1. Press cmd+p on macOS
1. Type `debug k2`
1. Press enter

## Getting Help

For support with our SaaS, [Kolide K2](https://app.kolide.com/?utm_source=oss&utm_medium=readme&utm_campaign=launcher),
please use the Intercom Help links inside that application, these are
floating in the lower right. Or, email support@kolide.co

For support regarding issues with our open-source projects, please
feel free to reach out to us in the osquery Slack team in the #kolide
channel, [invites are
here](https://join.slack.com/t/osquery/shared_invite/zt-h29zm0gk-s2DBtGUTW4CFel0f0IjTEw)
