# Uninstalling Osquery Launcher

> [!NOTE]
> This documents are for an open source launcher install. If you're looking for official instructions on how to uninstall
> Kolide Agent, try https://www.kolide.com/docs/using-kolide/agent/removal-instructions

> [!NOTE]
> All paths noted on this page are based off of the identifier used at the time of package creation.
> The paths listed here assume an identifier of `kolide`. If you use a different identifier you will
> need to update any paths accordingly

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

Directories:

- `/usr/local/kolide`
- `/var/kolide`
- `/etc/kolide`

## macOS

### Launcher (`kolide-osquery-launcher.pkg`)

Directories:

- `/usr/local/kolide`
- `/var/kolide`
- `/var/log/kolide`
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
sudo pkgutil --forget com.kolide.launcher
```
