# Launcher Auto Update Process

## Authors

- seph ([@directionless](https://github.com/directionless))

## Status

Accepted (March 11, 2019)

## Context

One of the features of Launcher is it's ability to securely update
osquery and itself. The unix implementation is a straightforward
`exec` implementation. However, Windows does not have an `exec`.

This ADR documents the current implementation, and a solution for
windows.

## Decision

New software versions are distributed using [The Update Framework
(TUF)](https://theupdateframework.github.io/). We use a Go client
library that we [built in-house](https://github.com/kolide/updater).

Launcher periodically checks for new updates. When a new version is
detected, it is downloaded into a staging directory, and then the
running binary is replaced. This code can be found in
[autoupdate.go](/pkg/autoupdate/autoupdate.go)

On Unix, launcher calls `syscall.Exec` to replace the current
executable without a new process, using the new binary. This code can
be found in [updater.go](/cmd/launcher/updater.go)

### Windows Variation

There are two needed changes on Windows.

First, Windows does not support replacing a running binary on
disk. Attempting will result in a `Permission Denied` error. A
workaround is to rename the old version, and then place them new one
in the correct location. This has the drawback of losing atomicity.

Second, Windows does not support `exec`. Instead, we will exit
launcher, and assume the service manager will restart. Empirically, it
will start the new binary on the configured path.

Exiting launcher is hard to navigate. Things inside TUF are buried
deep in routines. Simple returning an error isn't enough. While we
could call `os.Exit` that seems abrupt. So instead, we plumb the
signal channel through, and signal a `os.Interrupt` on it. (Note that
it's not a _signal_ in the posix sense. It's a constant sent to a
channel)

### Example Code

There an [example service](/tools/upgrade-exec-service-testing/)
exploring these mechanisms. See code for further comments and
discussion.

## Consequences

One consequence of this approach, is that the installed file is
_updated_ in place. While the new binary is verified by Launcher, this
may look like corruption in some packaging systems.

The update process on Windows is based on the service manager
restarting the service. We don't believe there's a downside here, but
it does increase restart counts. Due to implementation limitations of
WiX, the shortest recovery period is 1 day.

Due to the nature of this update process, updates that depend on
command line flag changes, require a re-installation of Launcher. They
are handled outside this update process.

The act of moving a new binary over a running old one (as we do on
unix) results in the running binary no longer being disk. This can
trigger notices in some monitoring software, for example, osquery.
