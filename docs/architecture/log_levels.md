# Log levels

Using "actor" here to describe a repeated or ongoing routine in launcher. It includes all rungroup actors, plus launcher itself, plus some other one-offs.

## What we need logs and log levels for

When troubleshooting, we want to be able to distinguish between and easily find the following categories of logs:

* Positive confirmation that launcher is operating as expected (routine actions are succeeding)
* Contextual status information (how launcher is performing at a given time)
* Error information (any time launcher tries to perform an action but cannot)
* Critical error information (any time an actor exits and must be restarted due to an error)

We want logs to be stored by level in different locations according to purpose:

* Stored locally and collected via flare; used to troubleshoot individual devices; may be more verbose
* Shipped to Cloud Log; used to troubleshoot devices across a tenant or multiple tenants, or to look for trends across a tenant or multiple tenants
* Shipped to Cloud Log and Error Reporting; used to identify errors and highlight trends that might indicate launcher bugs

## Proposal

We should use the same five levels we already have -- we don't need to add additional complexity by adding more, especially since we don't have more categories of logs to cover. We should use the following definitions to level our logs:

* LevelDebug: Routine information, such as ongoing status or performance
* LevelInfo: Normal but significant events, such as start up or shut down
* LevelWarn: Error occurred that is temporary or that does not prevent the action from continuing
* LevelError: Error occurred that prevents action from continuing
* LevelReportedError: Error occurred that results in actor shutdown

We could consider reporting "Error occurred that prevents _significant_ action from continuing" logs on the LevelReportedError level.

We should do the following with respect to log storage:

* We should continue to ship logs at LevelInfo and above to Cloud Log
* We should consider increasing the amount of logs that we keep locally, as sometimes we don't have enough history available there to troubleshoot

### Example categorized logs in launcher

I went through a selection of logs, categorized them, and then applied our new levels to them:

| Level | Category description |
|---|---|
| LevelDebug | Status info |
| LevelDebug | Configuration info |
| LevelDebug or LevelInfo | Successfully performed action |
| LevelInfo | Actor startup |
| LevelInfo | Actor shutdown |
| LevelInfo | Launcher detected error state and is performing self-corrective action |
| LevelWarn | Could not create non-critical actor, proceeding without it |
| LevelWarn | Potentially temporary error performing action, will retry |
| LevelWarn | Error performing action, can proceed |
| LevelError | Error performing action, cannot proceed |
| LevelReportedError | Actor exited unexpectedly |
| LevelReportedError | Actor panicked |

## Reference

### Other log level definitions

#### OpenTelemetry

| SeverityNumber range | Range name | Meaning |
|---|---|---|
| 1-4   | TRACE | A fine-grained debugging event. Typically disabled in default configurations. |
| 5-8   | DEBUG | A debugging event. |
| 9-12  | INFO  | An informational event. Indicates that an event happened. |
| 13-16 | WARN  | A warning event. Not an error but is likely more important than an informational event. |
| 17-20 | ERROR | An error event. Something went wrong. |
| 21-24 | FATAL | A fatal error such as application or system crash. |

[Source](https://opentelemetry.io/docs/specs/otel/logs/data-model/#field-severitynumber).

#### Golang (slog)

```golang
// Names for common levels.
//
// Level numbers are inherently arbitrary,
// but we picked them to satisfy three constraints.
// Any system can map them to another numbering scheme if it wishes.
//
// First, we wanted the default level to be Info, Since Levels are ints, Info is
// the default value for int, zero.
//
// Second, we wanted to make it easy to use levels to specify logger verbosity.
// Since a larger level means a more severe event, a logger that accepts events
// with smaller (or more negative) level means a more verbose logger. Logger
// verbosity is thus the negation of event severity, and the default verbosity
// of 0 accepts all events at least as severe as INFO.
//
// Third, we wanted some room between levels to accommodate schemes with named
// levels between ours. For example, Google Cloud Logging defines a Notice level
// between Info and Warn. Since there are only a few of these intermediate
// levels, the gap between the numbers need not be large. Our gap of 4 matches
// OpenTelemetry's mapping. Subtracting 9 from an OpenTelemetry level in the
// DEBUG, INFO, WARN and ERROR ranges converts it to the corresponding slog
// Level range. OpenTelemetry also has the names TRACE and FATAL, which slog
// does not. But those OpenTelemetry levels can still be represented as slog
// Levels by using the appropriate integers.
const (
	LevelDebug Level = -4
	LevelInfo  Level = 0
	LevelWarn  Level = 4
	LevelError Level = 8
)
```

#### GCP

| Level | Meaning |
|---|---|
| DEFAULT 	(0) 	| The log entry has no assigned severity level. |
| DEBUG 	(100)	| Debug or trace information. |
| INFO 		(200)	| Routine information, such as ongoing status or performance. |
| NOTICE 	(300)	| Normal but significant events, such as start up, shut down, or a configuration change. |
| WARNING 	(400)	| Warning events might cause problems. |
| ERROR 	(500)	| Error events are likely to cause problems. |
| CRITICAL 	(600)	| Critical events cause more severe problems or outages. |
| ALERT 	(700)	| A person must take an action immediately. |
| EMERGENCY (800)	| One or more systems are unusable. |

[Source](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#logseverity).
