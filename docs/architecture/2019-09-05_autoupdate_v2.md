# Launcher Auto Update Process Version 2

## Authors

- seph ([@directionless](https://github.com/directionless))

## Status

Accepted: October, 2019

Supersedes: [Launcher Auto Update Process](2019-03-11_autoupdate.md)

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

### Assumptions in TUF

Additionally, our implementation must work within the assumptions of
[TUF](https://godoc.org/github.com/kolide/updater/tuf).

TUF has a very simple model. It's designed to notice remote metadata
changes for a single file, download it into a given location, and
trigger into a callback function.

Because it has a single file model, we cannot easily store these by
version.

Furthermore, updates happen when the tuf metadata changes. Not when on
a binary mismatch. This means that if the local `launcher` executable
changes, it will not be refreshed until the tuf metadata
changes. This makes testing somewhat harder.

## Decision

Instead of replacing the running binary, we will create a directory to
store updates in. These updates will be launched from there. This new
flow will look like:

1. a new binary is downloaded into the staging area
2. It is moved into the updates directory, and stored by date
3. It is spawned from the updates directory


The implementation is documented as part of [/pkg/autoupdate], or
[godoc](https://godoc.org/github.com/kolide/launcher/pkg/autoupdate)

## Consequences

Because we're not replacing the binary on disk, if we want _all_
execution to use an update, we need to hook into the main
function. Thus all subsequent executions find the latest download.

To support this we need to store the updates in a configuration
agnostic fashion. We will use `<binaryPath>-updates`.

We will need to remove old updates. This cleanup routine can run as
part of finding the current binaries.
