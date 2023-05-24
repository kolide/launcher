## Library lookup

When launcher looks for the version to run for itself or for osquery, it first
looks through local TUF metadata to see if it knows what version to run for its
given release channel. If it does, and the version is already downloaded, it
will run that version.

Otherwise, it will look for the most recent version downloaded to its update
library.

```mermaid
flowchart TB
    A[Library lookup] --> B{Do we have a local TUF repo?}
    B ---->|No| C[Get most recent version from update library]
    C --> D[Return path to most recent version of executable]
    D --> H[End]
    B -->|Yes| E{release.json target metadata exists?}
    E -->|No| C
    E --> |Yes| F{Target indicated by release.json\nis downloaded to update library?}
    F --> |Yes| G[Return path to selected executable in update library]
    F --> |No| C
    G --> H
```
