```mermaid
sequenceDiagram
    Launcher->>+K2: flare request
    K2->>+GCP: create pre-signed url
    GCP-->>+K2: pre-signed url
    K2-->>+Launcher: pre-signed url
    Launcher->>+Launcher: run flare
    Launcher->>+GCP: upload flare zip with pre-signed url
```
