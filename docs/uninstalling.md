# Uninstalling Osquery Launcher

## Linux

### Debian

#### Installed via `dpkg`

These instructions cover uninstalling if you installed the offical package `kolide-osquery-launcher.deb` via `dpkg -i`.

To list packages installed via `dpkg` execute `dpkg --list`. The Osquery Launcher is called `launcher`, executing `dpkg --list | grep 'launcher'` will show the relevant entry.

To remove `launcher` and **preserve** configuration files execute `sudo dpkg --remove launcher`. 

To remove `launcher` and **remove** configuration files exeute `sudo dpkg --purge launcher`.

`dpkg --purge` will not delete directories which are not empty. As a result you might see a warning which looks like: 

```
dpkg: warning: while removing launcher, directory '/foo/bar/kolide/bin' not empty so not removed
dpkg: warning: while removing launcher, directory '/dir/kolide/' not empty so not removed
```
The left over directories mentioned in the `dpkg` warning can be removed with `sudo rm -rf`
