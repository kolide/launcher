# Launcher Autoupdate Process, Version 3

## Authors

- Rebecca Mahany-Horton ([@RebeccaMahany](https://github.com/RebeccaMahany))

## Status

Accepted: October 2023

Changes rolled out to nightly channel in October 2023.

Changes rolled out to beta channel in November 2023.

Slow rollout to stable channel began January 2024.

Supersedes: [Launcher Auto Update Process Version 2](2019-09-05_autoupdate_v2.md)

## Context

Our current autoupdate process has several areas we'd like to improve upon.

First, updates are currently stored on the end user device under the timestamp
when they were downloaded. This does not allow launcher to know what version
the update corresponds to.

Second, release channels are currently implemented by copying the most recent
version of an autoupdatable binary to `<binary>-<channel>.tar.gz` and publishing
this copy to Notary. We would like a more flexible implementation that does not
require storing a binary twice.

Third, we would like the location of updates to be configurable.

Fourth, we want to remove our reliance on Notary and use the [go-tuf](https://github.com/theupdateframework/go-tuf)
implementation of TUF instead.

See [Begin work on Autoupdate: The Next Generation](https://github.com/kolide/launcher/issues/954)
for more details.

## Decision

We created a new update directory that lives, by default, in the root
directory, but its location is configurable via flag.

Under the new system, launcher retrieves the latest build for its release
channel by inspecting the custom metadata stored on the known target
`<binary>/<os>/<arch>/<channel>/release.json`. This metadata points to the
target that should be downloaded. The target's filename contains the version,
taking the format `<binary>/<os>/<arch>/<binary>-<version>.tar.gz`. This allows
us to store only `release.json` per-channel instead of a copy of the tarball.
It additionally allows launcher to know the version of the binary that it's
downloading.

After downloading and verifying, launcher stores the update in the update
directory under the version. This allows launcher to perform version selection
based on the release.json target, rather than always picking the update with
the most recent timestamp. This will give us the ability to roll back releases
more quickly when needed -- if launcher has version 1.2.3 and 1.2.4 in its
library and we roll back stable from 1.2.4 to 1.2.3, launcher can switch to
running its local 1.2.3 update immediately, without having to perform a new
download to fetch newest stable.

The new system also uses the [go-tuf client](https://github.com/theupdateframework/go-tuf/tree/master/client),
rather than our previous [client implementation](https://github.com/kolide/updater),
to perform metadata download and verification.

## Consequences

The legacy autoupdater discovers the location of the legacy update directory
based on the current executable's path. Putting new updates in a different
directory results in the legacy autoupdater putting its updates in an incorrect
location. As a result, I've chosen to not run the legacy autoupdater alongside
the new autoupdater once the new autoupdater is used for version selection.
This results in a riskier rollout.

I've attempted to mitigate the risk of cutting over to the new system in
several ways:

1. Store errors that occur during the new autoupdate process in the
`kolide_tuf_autoupdater_errors` table; review and address unexpected errors.
1. Perform a rollout to nightly only, and then to nightly and beta, to test the
new system with a limited number of devices.
1. Perform a gradual rollout to stable.
1. Expand the TUF checkup in flare to fetch data about local and remote state,
to enable troubleshooting for autoupdate issues.
1. Add an automated test suite that exercises and validates autoupdate
functionality.
