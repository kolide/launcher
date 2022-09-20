# Live Debugging with VS Code

## Prerequisites

1. [Install VS Code](https://code.visualstudio.com/download)
1. [Install VS Code Go Extension](https://code.visualstudio.com/docs/languages/go)
1. Osqueryd is available in your path or a common location
1. Open launcher repo with VS Code
1. Copy ./tools/vscode-debug/conf to ./.vscode (`mkdir -p ./.vscode && cp -r ./tools/vscode-debugging/conf/. ./.vscode`)
* if this is your first time using the VS Code go extension, you'll be prompted to install various go packages when you start debugging

## Debugging With Osquery Interactive

1. Press cmd+p on macOS
1. Type `debug interactive`
1. Press enter

Now you should be able to set break points in VS Code and hit them by executing queries.

## Debugging Against Local K2 Server (only available to Kolide employees)

### First time setup

1. Log into your local instance of K2 > Inventory > Add Device, the enroll_secret will be displayed in the launcher command
1. Save your enroll_secret to `./debug/k2_enroll_secret`
1. Either copy the {k2-repo}/tmp/localhost.crt to `./debug/localhost.crt` or create a sym like from `{k2-repo}/tmp/localhost.crt` -> `./debug/localhost.crt` at the root of your launcher repository
   ```sh
   # symlink cmd
   ln -s <k2-repo>/tmp/localhost.crt <launcher-repo>/debug/localhost.crt
   ```
### After first time setup

1. Start K2 locally
1. Open launcher repo with VS Code
1. Press cmd+p on macOS
1. Type `debug k2`
1. Press enter