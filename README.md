The Osquery Launcher ![CircleCI](https://circleci.com/gh/kolide/launcher.svg?style=svg&circle-token=e33dd9f3fec934f64b17e15c68ed57209f61117e)
====================

The Osquery Launcher is a lightweight launcher/manager which offers a few extra capabilities on top of osquery:

- secure automatic updates of osquery
- remote communication via a modern gRPC server API
- a curated `kolide_best_practices` table which includes a curated set of standards for the modern enterprise
- tooling to generate deployment packages for a variety of platforms

[![osquery is lightweight](./tools/images/lightweight.png)](https://kolide.co/osquery)

### Documentation

The documentation for this project is included on GitHub in the [`docs`](./docs/README.md) subdirectory of the repository.

## Features

### Secure Osquery Autoupdater

Osquery is statically linked and that allows for the easy bundling and distribution of capabilities. Unfortunately, however, it also implies that you have to maintain excellent osquery update hygiene in order to take advantage of emerging osquery capabilities.

The Launcher includes the ability to securely manage and autoupdate osquery instances. This is implemented using [The Update Framework (TUF)](https://theupdateframework.github.io/). TUF defines a [specification](https://github.com/theupdateframework/tuf/blob/develop/docs/tuf-spec.md) for secure software update systems. The spec describes a client/server model where the client is the software to be updated and the server is the update server. For our implementation, we use [Docker Notary](https://github.com/docker/notary) as our TUF server and a Go client library that [we built in-house](https://github.com/kolide/updater).

Because we understand the security implications of an osquery autoupdater, NCC Group was contracted to perform a security audit of our in-house TUF client library. This report is [available for public review](https://www.nccgroup.trust/globalassets/our-research/us/public-reports/2017/ncc-group-kolide-the-update-framework-security-assessment.pdf). NCC Group has also previously performed assessments on [Docker Notary](https://www.nccgroup.trust/us/our-research/docker-notary/) and [Osquery](https://www.nccgroup.trust/us/about-us/newsroom-and-events/blog/2016/march/ncc-group-reviews-osquery/) as well.

### gRPC Server Specification and Implementation

Osquery has a very extensible plugin architecture that allow it to be heavily customized with plugins. The included TLS plugins are used by many existing osquery management servers, but the design of the TLS API leaves much to be desired. The Launcher includes a set of gRPC plugins for remote communication with a gRPC server. The [server specification](https://github.com/kolide/launcher/tree/master/pkg/pb/launcher) is independently published and versioned.

### Kolide's Best Practices

Osquery allows you to ask a lot of great questions, but sometimes it's hard to know exactly which questions you should ask and what queries will expose the answers. The Launcher includes a table called `kolide_best_practices` which aggregates useful information in an easy "compliant" vs "not compliant" interface. Consider the following queries:

-	`select gatekeeper_enabled from kolide_best_practices`
- `select remote_login_disabled from kolide_best_practices`
- ~`select screensaver_password_enabled from kolide_best_practices`~ see https://blog.kolide.com/screensaver-security-on-macos-10-13-is-broken-a385726e2ae2

The following best practices, and many more, are included:

- Is SIP enabled?
- Is Filevault enabled?
- Is the firewall enabled?
- Are [Remote Apple Events](https://support.apple.com/kb/PH18721?locale=en_US) disabled?
- Is Internet Sharing disabled?

### Reduced Configuration Surface

The osqueryd binary was designed to be very configurable, which allows it to be used in very different environments. The Launcher wraps osqueryd configuration and exposes very high-level options that allow you to easily connect osquery to a server that is compliant with the [gRPC specification]

Consider the following side-by-side example of The Launcher's command-line help versus osqueryd's command-line help. The Launcher exposes the bare essentials as top-level configuration options. This makes getting started with Osquery easier than ever.

[![launcher is simple](./tools/images/help-side-by-side.png)](./docs/launcher.md)

To learn about The Launcher's command-line interface, see the Launcher [documentation](./docs/launcher.md).

### Easy Packaging and Deployment Tooling

Deploying osquery and configuring it to communicate with a management server can be complicated, especially if you have to make customized deployment packages. The Launcher includes a tool called `package-builder` which you can use to create Launcher packages for your organization.

To learn more about using `package-builder` to package and deploy osquery, check out the [documentation](./docs/package-builder.md).

## Kolide K2

Want to go directly to insights? Not sure how to package Launcher or manage your Fleet?

Try our [osquery SaaS platform](https://kolide.com/?utm_source=oss&utm_medium=readme&utm_campaign=launcher) providing insights, alerting, fleet management and user-focused security tools. We also support advanced aggregation of osquery results for power users. Get started immediately, with your 14-day free trial [today](https://kolide.com/signup?utm_source=oss&utm_medium=readme&utm_campaign=launcher). Launcher packages customized for your organization can be downloaded in-app after signup.
