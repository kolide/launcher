## Library lookup

When launcher looks for the version to run for itself or for osquery, it first
checks to see if a version has been pinned and has already been downloaded; if
it is set and available, it will run that version. If not, launcher looks
through local TUF metadata to see if it knows what version to run for its
given release channel. If it does, and the version is already downloaded, it
will run that version.

Otherwise, it will look for the most recent version downloaded to its update
library.

```mermaid
flowchart TB
    A[Library lookup] --> B{Is there a pinned version and is it available in the update library?}
    B ---->|Yes| C[Return path to pinned version of executable in update library]
    C --> D[End]
    B -->|No| E{Do we have local TUF data including release.json target metadata?}
    E -->|No| F[Return path to most recent version of executable in update library]
    F --> D
    E --> |Yes| G{Target indicated by release.json is downloaded to update library?}
    G --> |Yes| H[Return path to release version of executable in update library]
    G --> |No| F
    H --> D
```
