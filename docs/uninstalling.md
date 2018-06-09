# Uninstalling Osquery Launcher

## Linux

### Debian

#### Installed via `dpkg`

These instructions cover uninstalling if you installed the offical package `kolide-osquery-launcher.deb` via `dpkg -i`.

To list packages installed via `dpkg` execute `dpkg --list`. The Osquery Launcher is called `launcher`, executing `dpkg --list | grep 'launcher'` will show the relevant entry.

To remove `launcher` and **preserve** configuration files execute `sudo dpkg --remove launcher`. 

To remove `launcher` and **remove** configuration files exeute `sudo dpkg --purge launcher`.

`dpkg --purge` will not delete directories which are not empty. As a result you will likely see the following warning: 

```
dpkg: warning: while removing launcher, directory '/usr/local/kolide/bin' not empty so not removed
dpkg: warning: while removing launcher, directory '/var/kolide/dichiye.launcher.kolide.com-443' not empty so not removed
```

To remove these leftover directories execute:
- `sudo rm -rf /usr/local/kolide`
- `sudo rm -rf /var/kolide`

