```mermaid
sequenceDiagram
    participant LauncherKolideK2Svc
    Note right of LauncherKolideK2Svc: ./launcher.exe svc ...
    create participant WindowsServiceManager
    LauncherKolideK2Svc->>WindowsServiceManager: opens connection on startup
    create participant LauncherKolideRestartSvc
    WindowsServiceManager->>LauncherKolideRestartSvc: have we installed the restart service?
    Note left of LauncherKolideRestartSvc: ./launcher.exe restart-service

    alt yes the service already exists
        LauncherKolideK2Svc->>LauncherKolideRestartSvc: Restart to ensure latest
    else no the service does not exist
        LauncherKolideK2Svc->>WindowsServiceManager: 1 - create, configure, etc
        LauncherKolideK2Svc->>LauncherKolideRestartSvc: 2 - Start
        activate LauncherKolideRestartSvc
    end

    loop every n minutes
        LauncherKolideRestartSvc->>WindowsServiceManager: Query LauncherKolideK2Svc status
        LauncherKolideRestartSvc->>LauncherKolideK2Svc: Start if Stopped
    end
```