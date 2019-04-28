## Running with systemd

Once you've verified that you can run launcher in your shell, you'll likely want to keep launcer running in the background and after the endpoint reboots. To do that we recommend using [systemd](https://coreos.com/os/docs/latest/getting-started-with-systemd.html)

Below is a sample unit file.

```
[Unit]
Description=The Kolide Launcher
After=network.service syslog.service

[Service]
ExecStart= $LauncherPath \
  --hostname=$FleetServer:FleetPort \
  --enroll_secret=$FleetSecret \
  --autoupdate=true \
  --osqueryd_path=$OsquerydPath
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
```

Once you created the file, you need to move it to `/etc/systemd/system/launcher.service` and start the service.

```
sudo mv launcher.service /etc/systemd/system/launcher.service
sudo systemctl start launcher.service
sudo systemctl status launcher.service

sudo journalctl -u launcher.service -f
```

If running launcher for tests purposes, using local or insecure certificates, include the option `--insecure`.

## Making changes
Sometimes you'll need to update the systemd unit file defining the service. To do that, first open `/etc/systemd/system/launcher.service` in a text editor, and make your modifications.

Then, run

```
sudo systemctl daemon-reload
sudo systemctl restart launcher.service
```
