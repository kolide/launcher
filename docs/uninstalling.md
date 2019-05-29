# Uninstalling Osquery Launcher

## Linux

### Debian

#### Installed via `dpkg`

These instructions cover uninstalling a Launcher package (ie: `kolide-osquery-launcher.deb`) via `dpkg -i`.

To list packages installed via `dpkg` run the following:

```
dpkg --list
```

The Osquery Launcher is called `launcher`, executing `dpkg --list | grep 'launcher'` will show the relevant entry.

To remove `launcher` and **preserve** configuration files, run the following:

```
sudo dpkg --remove launcher
```

To remove `launcher` and **remove** configuration files, run the following:

```
sudo dpkg --purge launcher
```

`dpkg --purge` will not delete directories which are not empty. As a result you might see a warning which looks like: 

```
dpkg: warning: while removing launcher, directory '/usr/local/kolide/bin' not empty so not removed
dpkg: warning: while removing launcher, directory '/var/kolide/launcher.example.org-443' not empty so not removed
```

Based on the configurations used when the Launcher package was created, the specific paths printed may look slightly different. In any case, these left over directories mentioned in the `dpkg` warning can be removed with `sudo rm -rf`.

## macOS

### Launcher (`kolide-osquery-launcher.pkg`)

Directories:

- `/usr/local/kolide`
- `/var/kolide`
- `/etc/kolide`

Files:
- `/Library/LaunchDaemons/com.kolide.launcher.plist`

To remove the binaries and other supporting files, run the following:

```
sudo launchctl unload /Library/LaunchDaemons/com.kolide.launcher.plist
sudo rm /Library/LaunchDaemons/com.kolide.launcher.plist
sudo rm -r /usr/local/kolide
sudo rm -r /var/kolide
sudo rm -r /etc/kolide
```

### App (`kolide-desktop-app.pkg`)

Directories:
- `$HOME/Applications/Kolide.app`
- `"$HOME/Library/Application Support/Kolide"`
- `/usr/local/kolide`


Files:
- `/Library/LaunchDaemons/com.kolide.launcher.plist`

To remove the `.app` bundle, run the following:

```
sudo rm -r /Applications/Kolide.app
```

To remove the preferences, cache and other supporting files, run the following:

```
sudo rm -r "$HOME/Library/Application Support/Kolide"
sudo rm -r /usr/local/kolide
sudo launchctl unload /Library/LaunchDaemons/com.kolide.launcher.plist
sudo rm /Library/LaunchDaemons/com.kolide.launcher.plist
```
