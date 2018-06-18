# Sending initial host details during the enrollment request.

## Authors

- Victor Vrantchan ([@groob](https://github.com/groob))

## Status

Accepted (June 15, 2018)

## Context

When enrolling osquery into a remote server, it's desirable to also have a set of initial fields about the osquery node. Usually, servers like Fleet queue ad-hoc queries to get this information, but it's not always reliable. For example, a osquery host can succeed during the Enroll method, and then immediately crash, leaving the server operator with little context about which osquery host to troubleshoot. Another effect of populating host details via distributed query is that they can add some latency to the enrollment process depending on the query interval. 

## Decision

Add a EnrollmentDetails structure to the RequestEnrollment method which sends the results of a population query as part of the enrollment. Below is a list of chose attributes that launcher will always send:

```
message EnrollmentDetails {
    string os_version = 1;
    string os_build = 2;
    string os_platform = 3;
    string hostname = 4;
    string hardware_vendor = 5;
    string hardware_model = 6;
    string hardware_serial = 7;
    string osquery_version = 8;
    string launcher_version = 9;
}
```

## Consequences

Enrolling a host will always send the required data for a server to identify the host which is attempting enrollment.
To make this change, we had to re-order the position of the `/osquery/extension.Enroll` method in the launcher extension. Initially, the Enroll method was called even before osqueryd started, in order to ensure that when osqueryd starts up, there is a valid node_key to fetch the remote config. I had to use the osquery-go client to run the population query, which meant that osqueryd must already be running. Re-arranging the order of the components does not pose a visible issue, as the Enroll method is called immediately. In case of latency between osqueryd starting, and a node key being returned by the server, osqueryd wil fail to fetch a config. In practice, the `RequestConfig` method is retried immediately after `Enroll`, avoiding potential problems.

I also had to expose a `SetQuerier` method on the osquery extension, in order to allow setting the client.
```
// Querier allows querying osquery.
type Querier interface {
	Query(sql string) ([]map[string]string, error)
}
```

This method is implemented by the `runtime.Runner`. An unfortunate consequence of the re-enrollment APIs is that the signature of the `Enroll` method must not have additional argument, so I had to find a different way to set the client on the extension. When someone instantiates a `osquery.Extension`, they need to know to also call `extension.SetQuerier(runner)` in order for the EnrollmentDetails to be populated.
