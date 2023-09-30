# Revisiting enrollment details

This continues the work started in [Initial Host Details](2018-06-15_request_enrollment_details.md)

## Authors

- seph ([@directionless](https://github.com/directionless))

## Status

Accepted (2023-09)

## Context

Over the last several years, roughly since late 2021, we've seen
occasional problems starting up. Internally, we've called this _The
Monterey Bug_, and there is some information and links in [GitHub
Issue #1211(https://github.com/kolide/launcher/issues/1211).

We have never managed to diagnose this. We have reproduced small parts
of it, but nothing that holds up over all.

Our leading theories are that it is somehow related to runtime
complexity, osquery startup time, and thrift socket contention. There
may be multiple related and unrelated issues at play.

There is additional complexity that stems from the original
implementation of
[`getEnrollmentDetails`](https://github.com/kolide/launcher/blob/ab411f07d1d147b963809df2e1fdb04cb574d1a3/pkg/osquery/extension.go#L934). Because
it use the osquery socket, it cannot run until osquery is started. But
simultaneously launcher is trying to register extensions, and osquery
is trying to enroll. 

## Decision

To both simplify startup ordering _and_ reduce socket contention, we
can gather enrollment details via execing osquery. Semantically, this
is a fairly simple change -- we can use the same query, and the same
osquery.

## Consequences

We incur an exec call during startup. But in return, we can gain
several benefits: 
- Decouple enrollment details from the main osquery startup.
- Reduce contention on the socket during early startup
- Enrollment no longer has a circular dependency
- Enables future work to completely pull enrollment into launcher 
