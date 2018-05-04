# Add CRC and UUID to log lines

## Authors

- Nick Esposito ([@nicktitle](https://github.com/nicktitle))

## Status

Proposed

## Context

**TL;DR**

A lack of unique info per-log line makes it hard to uniq osquery data downstream, especially in cases when distinct log lines with identical data can be generated. To improve visibility, we should add a UUID to each emitted line. Additionally, adding a hash of the data lets you uniq log line data without inspecting all the fields of these log lines.

**Detail**

While a unique ID is generated for every batched log event emitted from launcher, individual log lines within that log cannot be determined to be unique. This is both true for distinct logs which are identical in their contents, and in the instance that the downstream data pipeline cannot guarantee "exactly once" delivery of messages.

By including a UUID for each log line, distinct but semantically identical log lines of data can be determined not to be duplicates.

Additionally, by including a CRC of the data inside each log line, downstream consumers of the log can identify log lines with identical data without inspecting the entirety of log line's data.

Note that the crc must be calculated before adding the uuid, or else it won't be useful


## Decision

Launcher should unpack logs from osquery and decorate each line with both a CRC of the data, and a UUID.


## Consequences

### Data Integrity

Launcher would be decorating the actual log data with foreign fields, not native to the actual osquery results. Not all users of Launcher may want this, so we'd need to be able to disable it as well

### Latency

Unpacking logs received from osquery, decorating them, and packing them again will add latency to each log sent out of Launcher. This should be a small amount of time, but it adds a processing burden and additional time to each request leaving the host.


## Tags

```
launcher,duplicates,crc,uuid,logs,
```
