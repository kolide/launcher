The Kolide Osquery Launcher
===========================

The Kolide Osquery Launcher is a lightweight launcher/manager which
offers a few extra capabilities on top of osquery:

- secure automatic updates of osquery
- many additional tables
- tooling to generate deployment packages for a variety of platforms

[![osquery is lightweight](./tools/images/lightweight.png)](https://kolide.co/osquery)

### Documentation

The documentation for this project is included on GitHub in the
[`docs`](./docs/README.md) subdirectory of the repository.

## Major Features

### Secure Osquery Autoupdater

Osquery is mostly statically linked and this allows for the easy
bundling and distribution of capabilities. Unfortunately, however, it
also implies that you have to maintain excellent osquery update
hygiene in order to take advantage of emerging osquery capabilities.

The Launcher includes the ability to securely manage and autoupdate
osquery instances. This is implemented using [The Update Framework
(TUF)](https://theupdateframework.github.io/). TUF defines a
[specification](https://github.com/theupdateframework/tuf/blob/develop/docs/tuf-spec.md)
for secure software update systems. The spec describes a client/server
model where the client is the software to be updated and the server is
the update server. For our implementation, we use [Docker
Notary](https://github.com/docker/notary) as our TUF server and a Go
client library that [we built
in-house](https://github.com/kolide/updater).

Because we understand the security implications of an osquery
autoupdater, NCC Group was contracted to perform a security audit of
our in-house TUF client library. This report is [available for public
review](https://www.nccgroup.trust/globalassets/our-research/us/public-reports/2017/ncc-group-kolide-the-update-framework-security-assessment.pdf). NCC
Group has also previously performed assessments on [Docker
Notary](https://www.nccgroup.trust/us/our-research/docker-notary/) and
[Osquery](https://www.nccgroup.trust/us/about-us/newsroom-and-events/blog/2016/march/ncc-group-reviews-osquery/)
as well.

### Additional Tables

Osquery exposes a lot of information, but there is always
more. Launcher includes all of the Kolide tables exposing a wealth of
additional information.

### Reduced Configuration Surface

The osqueryd binary was designed to be very configurable, which allows
it to be used in very different environments. The Launcher wraps
osqueryd configuration and exposes very high-level options that allow
you to easily connect osquery to a server that is compliant with the
gRPC specification

To learn about The Launcher's command-line interface, see the Launcher
[documentation](./docs/launcher.md).

### Easy Packaging and Deployment Tooling

Deploying osquery and configuring it to communicate with a management
server can be complicated, especially if you have to make customized
deployment packages. The Launcher includes a tool called
`package-builder` which you can use to create Launcher packages for
your organization.

To learn more about using `package-builder` to package and deploy
osquery, check out the [documentation](./docs/package-builder.md).

## Kolide K2

Want to go directly to insights? Not sure how to package Launcher or
manage your Fleet?

Try our [osquery SaaS
platform](https://kolide.com/?utm_source=oss&utm_medium=readme&utm_campaign=launcher)
providing insights, alerting, fleet management and user-focused
security tools. We also support advanced aggregation of osquery
results for power users. Get started immediately, with your 14-day
free trial
[today](https://k2.kolide.com/trial/new?utm_source=oss&utm_medium=readme&utm_campaign=launcher). Launcher
packages customized for your organization can be downloaded in-app
after signup.
