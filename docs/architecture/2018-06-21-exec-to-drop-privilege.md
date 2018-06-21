# Exec to drop privilege and query CoreFoundation preferences.

## Authors

- Victor Vrantchan ([@groob](https://github.com/groob))

## Status

Accepted (June 21, 2018)

## Context

Certain macOS preferences can only be read when queried reliably through the user context. We ran into this issue trying to query the `DiscoverableMode` key for Airdrop. 
Using `setuid` to set the user context works, but on posix systems setuid applies to the entire process instead of a single thread.
There is also a `pthread_setugid_np` API intended to set credentials for a single thread, but that solution did not appear to work in this case. The CoreFoundation APIs would still execute as root.

```
void Prefs() {
      pthread_setugid_np(501, 20);
      NSUserDefaults * userDefaults = [[NSUserDefaults alloc] initWithSuiteName:@"com.apple.sharingd"];
      NSString * str = [userDefaults stringForKey:@"DiscoverableMode"];
      NSLog(@"%@", str);
}
```

Relevant Quote from Apple:
```
https://developer.apple.com/library/archive/technotes/tn2083/_index.html#//apple_ref/doc/uid/DTS10003794-CH1-SUBSECTION38

It is not possible for a daemon to act on behalf of a user with 100% fidelity. While this might seem like a controversial statement, it's actually pretty easy to prove. For example, consider something as simple as accessing a preference file in the user's home directory. It's not possible for a daemon to reliably do this. If the user has an AFP home directory, or their home directory is protected by FileVault, the volume containing the home directory will only be mounted when the user is logged in. Moreover, it is not possible to mount the that volume without the user's security credentials (typically their password). So, if a daemon tries to get a user preference when the user is not logged in, it will fail.

In some cases it is helpful to impersonate the user, at least as far as the permissions checking done by the BSD subsystem of the kernel. A single-threaded daemon can do this using seteuid and setegid. These set the effective user and group ID of the process as a whole. This will cause problems if your daemon is using multiple threads to handle requests from different users. In that case you can set the effective user and group ID of a thread using pthread_setugid_np. This was introduced in Mac OS X 10.4.
```

## Decision

Implement a new entrypoint for the `launcher` and `launcher.ext` binaries which can be called in an exec with the `cf_preference key domain` arguments.

## Consequences

Querying `cf_preferences` using an exec in certain circumstances allows reliably getting the value for the user, but at a increased cost. Running the query at a less frequent interval can mitigate the cost of the exec. 
For a preference like `DiscoverableMode` which is set by the user in the Airdrop UI in Finder, a increased query interval would result in missed events. 
A possible solution to explore long term would be to create an [evented table](https://osquery.readthedocs.io/en/stable/development/pubsub-framework/) for user defaults with a running process in the console user context.

## Notes:
- [Osquery Issue: joining users table has no effect on preferences table](https://github.com/facebook/osquery/issues/4244)
