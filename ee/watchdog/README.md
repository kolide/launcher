### Watchdog Service

Here is a basic sequence diagram displaying the enable path for the windows watchdog service. The `launcher_watchdog_enabled` control flag will trigger the initial configuration and installation, and removal of the flag will trigger removal of the service.

```mermaid
sequenceDiagram
    participant LauncherKolideK2Svc
    Note right of LauncherKolideK2Svc: ./launcher.exe svc ...
    create participant WindowsServiceManager
    LauncherKolideK2Svc->>WindowsServiceManager: if launcher_watchdog_enabled
    create participant LauncherKolideWatchdogSvc
    WindowsServiceManager->>LauncherKolideWatchdogSvc: have we installed the watchdog?
    Note left of LauncherKolideWatchdogSvc: ./launcher.exe watchdog

    alt yes the service already exists
        LauncherKolideK2Svc->>LauncherKolideWatchdogSvc: Restart to ensure latest
    else no the service does not exist
        LauncherKolideK2Svc->>WindowsServiceManager: 1 - create, configure, etc
        LauncherKolideK2Svc->>LauncherKolideWatchdogSvc: 2 - Start
        activate LauncherKolideWatchdogSvc
    end

    loop every n minutes
        LauncherKolideWatchdogSvc->>WindowsServiceManager: Query LauncherKolideK2Svc status
        LauncherKolideWatchdogSvc->>LauncherKolideK2Svc: Start if Stopped
    end
```

The restart functionality is currently limited to detecting a stopped state, but the idea here is to lay out the foundation for more advanced healthchecking.
The watchdog service itself runs as a separate invocation of launcher, writing all logs to sqlite. The main invocation of launcher runs a watchdog controller, which responds to the `launcher_watchdog_enabled` flag, and publishes all sqlite logs to debug.json.