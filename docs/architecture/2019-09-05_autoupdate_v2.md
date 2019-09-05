
## Authors

- seph ([@directionless](https://github.com/directionless))

## Status

Proposed

Supersedes [Launcher Auto Update Process](2019-03-11_autoupdate.md)

## Context

Our current update process has several flaws. These lead towards an
unreliable autoupdate process.

The existing autoupdate functionality works as follows:

On posix:
1. A new version is downloaded into the staging area
2. The new version is moved to replace the current binary
3. The new binary is exec'ed

On Windows:
1. A new version is downloaded into the staging area
2. The current exe is moved out of the way
3. The new exe is moved into place
4. The service is restarted

There are some inherent problems with this design:
* Running binaries no longer appear on disk
* Files on disk no longer match what was installed via the package manager
* On windows, there is a failure condition where the new binary isn't
  moved into place, and the next service restart causes a service
  failure (https://github.com/kolide/launcher/issues/509)

These lead us to needing a new update process.

## Decision

Instead of replacing the running binary, we will launch the updated
binary from a staging area. This new flow will look like:

1. a new binary is downloaded into the staging area
2. It is moved into the updates directory
3. It is spawned from the updates directory 

The implementation is documented as part of [/pkg/autoupdate], or
[godoc](https://godoc.org/github.com/kolide/launcher/pkg/autoupdate)

There are several corollaries:

* When launcher starts, it needs to look for a newer version to spawn
* Some cleanup process occurs to keep the number of binaries low. 

Some of this design stems from assumptions made by
[TUF](https://godoc.org/github.com/kolide/updater/tuf)

## Consequences


### Working around deficiencies in TUF

TUF is has some stuff baked pretty deep about writing to a staging
path, and no more. I'm not sure I want to change that so deeply.

Furthermore, updates happen when the tuf metadata changes. Not when on
a binary mismatch. This means that if the local `launcher` executable
changes, it will not be refreshed until the tuf metadata
changes. (This is contrary to the expectation that that the update
cycle would notice and repair it)
