# Control Server Initial API

## Authors

- Mike Arpaia ([@marpaia](https://github.com/marpaia))
- Logan McPhail ([@loganmac](https://github.com/loganmac))

## Status

- Proposed (July 06, 2018)

## Context

Osquery provides a client-side implementation to a [remote API](https://osquery.readthedocs.io/en/stable/deployment/remote/) that allow a server that implements a given API specification to perform various remote configuration and control objectives. Osquery is designed with the design objective to be minimal and read-only. This is why, for example, Launcher supports osquery autoupdate: because such a feature would be inappropriate in osquery core. Some users are willing to accept the risk involved with enabling autoupdate for osqueryd, so they enable this opt-in feature of Launcher by setting the `--autoupdate` flag when invoking Launcher.

Similarly, some users are willing to accept the risk of "writable" features. We see this in the osquery community with things like writable extensions, where users are able to enable extensions that allow for the mutation of table state and action is taken as a result. See [Mike Myers' QueryCon 2018 talk](https://www.youtube.com/watch?v=g46rjoP18EE) for more information on writable extensions.

Not all use-cases map well to a tabular expression mechanism however. For these features, we are proposing an extension to the osquery remote API specification for certain "writeable" features. This rest of this document outlines the details of the API, some initial (alpha) request formats, and the implications of this API existing.

## Current Decision

This document endeavors to describe an API extension to the osquery remote API which provides features and capabilities that are not appropriate to be included in upstream osquery. The first feature we're exploring adding to this "control API" is a web-based remote shell feature. The initial implementation of this feature is loosely based on the work done by the [GoTTY project](https://github.com/yudai/gotty). This capability has been adapted to use the same authentication mechanism as the existing osquery client in a consistent-feeling API.

The initial API is a TLS API but we may be exploring a gRPC version as well soon. The initial request and response types are documented below. Note that these are for illustrative purposes only and that this API will receive rapid iteration based on feedback from users and the community.

### Get Shells API

The "get shells" request is similar in structure to [osquery's distributed read request](https://osquery.readthedocs.io/en/stable/deployment/remote/#remote-server-api). The body of the POST request should contain JSON with one top-level key: `node_key`.

```json
{
  "node_key": "2ad06573-3082-4e6f-8fcc-06f1be3b6fa7"
}
```

The response may contain the following keys:

- `sessions`
- `error`
- `node_invlaid`

If `error` is present, an error occurred processing the request. If `node_invalid` is present, the host must re-enroll. If `sessions` is present, it will contain a list of shell sessions that the host should launch. With each session request, an additional one-time secret is also used to further authenticate the request.

```json
{
  "sessions":
     [
       {
         "session_id": "991464f3-2745-4034-bcb7-64143422cd19",
         "secret": "0bf30053-d699-4808-a177-74a56946b181"
       }
     ],
  "error": "",
  "node_invalid": false
}
```

### Join Shell Session API

As a result of receiving requests to start a shell session, *the Launcher will exec /bin/bash* and attach the TTY to a websocket connection for the supplied `session_id`:

```bash
curl -X POST \
  http://launcher.example.com/api/shells/991464f3-2745-4034-bcb7-64143422cd19 \
  -H 'Content-Type: application/json' \
	-H 'Authorization: Bearer 0bf30053-d699-4808-a177-74a56946b181'
  -d '{}'
```

An operator would presumably also establish a connection to the server, which is acting as a websocket broker. When the operator exits the web-based shell experience, the connection is closed for the client.

## Consequences

It's important to be explicit that this is a completely opt-in feature. Similarly to autoupdate, if you do not go out of your way to enable this in the Launcher via the `--control` flag, the entire control subsystem is ignored. If you are a user of the Launcher for it's extra tables, it's osquery runtime, or it's osquery autoupdate capabilities, but do no want control server features, simply don't enable them and your experience won't change.
