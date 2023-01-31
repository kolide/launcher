# Secure enclave, notifications, user context, and entitlements

31 Jan 2023.

## Problem

When testing launcher by running a signed build with sudo, it can access the secure enclave; it can also spawn a desktop process as a user that is able to send notifications. When launcher runs as a daemon instead, as it does normally, it is unable to access the secure enclave and the desktop process it spawns is not able to send notifications. This is because launcher cannot find the necessary services to talk to, because it does not have access to the user bootstrap context, ultimately resulting in an [NSXPCConnectionInvalid](https://osstatus.com/search/results?platform=all&framework=all&search=4099) error.

To allow for sending notifications, we must update how we run the desktop process so that it is running in the correct user context. To allow for secure enclave access, we must either move secure enclave access to a user desktop process running in the correct context, or we must find another method of secure enclave access that does not rely on the user context. It looks very likely that our only reasonable option for moving forward with secure enclave access is to move it into the user context.

## Potential solutions

### Find a new way to access the secure enclave

Solutions documented below involve moving secure enclave access out of the root launcher process and into the user desktop process, and then altering how we run that desktop process. After further research, it looks like it will not be possible for us to do this any other way – documentation indicates the only way to access the secure enclave is from the data protection keychain, which we cannot access in the daemon process, because it requires the user context; we cannot access the secure enclave from the file-based keychain.

It looks possible to me that we could use the command-line tool `/usr/libexec/seputil` to create and fetch a key. However, it is not very well documented, so I think it might be slow going to figure out if it’s even possible to use seputil to get around the limitation mentioned above.

We currently use the Security framework to access the secure enclave. CryptoKit is another framework that allows for secure enclave access, but this is a poor substitute for us because a) it falls back to the Security framework for access, and b) it is written in Swift, which would require more work to integrate with. Those are the only options documented [here](https://developer.apple.com/documentation/security/certificate_key_and_trust_services/keys/protecting_keys_with_the_secure_enclave?language=objc).

#### Pros

* We would probably prefer to not move secure enclave access into the user desktop process; this would allow us to avoid that

#### Cons

* It does not seem likely that this option is possible and it would probably take a while to figure out how to use seputil appropriately to confirm

### Move secure enclave access to the desktop process; run the desktop process as an agent

Instead of spawning the desktop process in launcher with an exec, we could instead create a LaunchAgent plist in the desired user’s ~/Library/LaunchAgent directory and let launchd handle it. This will start up the launcher desktop process in the user’s login context. In testing, this allowed the desktop process both to send notifications and to access the secure enclave.

I tested with an Aqua session type because that’s the default. It’s possible we could use Background instead.

We could move secure enclave access into the desktop process, or create a separate process in the user context to handle secure enclave access.

#### Pros

* It works for both secure enclave and notifications
* This is what Apple recommends as the correct path forward for a background process that has access to the user’s login context; they specifically warn that not running user processes in the correct namespace and context will likely raise obscure and difficult-to-solve issues in the future
* This is something we wanted to do anyway
* This likely has the side effect of making the annoying “The connection to service named com.apple.fonts was invalidated” logs go away, but I didn’t confirm this

#### Cons

* This option is probably the most amount of work, requiring:
  * Net-new code to manage a launch agent – creating it, modifying it, stopping it, removing it
  * Rewrite at least some of the current launcher root to desktop process communication to implement whatever’s necessary for XPC between the launcher root process and the launch agent process
  * We either have to assign ports safely to all of the launch agent processes, or do what Apple also recommends and swap our client/server model so that the daemon process is the server and the desktop agents are the clients
  * Move secure enclave access to the desktop process
  * Update launcher to talk to the desktop process when it needs access to the secure enclave
* When the launcher desktop process runs as a launch agent, the end user will get a popup allowing them to manage when it runs in Login Items
* We probably do not want to move secure enclave access into the desktop process if we can help it

### Run the desktop process with `launchctl asuser`, and then figure out entitlements

Instead of spawning the desktop process in launcher by directly calling the launcher binary as the user, we can run `launchctl asuser <uid> <launcher binary> desktop …`. This will execute launcher in the user’s login context. This permits us to send notifications, but it does not fix secure enclave access, although we get slightly farther: we then run into [errSecMissingEntitlement](https://osstatus.com/search/results?platform=all&framework=all&search=-34018). (Attempting to add additional entitlements, like keychain-access-groups, breaks both notifications and secure enclave access entirely.) From this info, it seems like `launchctl asuser` correctly runs launcher desktop in a context where it can access the services it needs, but it’s not correctly picking up on the entitlements.

We could investigate more to see if we can figure out the entitlements issue.

In the short term, we are moving to run with `launchctl asuser` to achieve the notifications fix.

#### Pros

* This requires less of a rewrite than the previous option

#### Cons

* This does not fix secure enclave access unless we can figure out why the `launchctl asuser` process is not inheriting entitlements correctly

## Resources and references

* [Forum post answer indicating that the secure enclave cannot be accessed from a daemon](https://developer.apple.com/forums/thread/115833)
* [Forum post answer indicating that secure enclave integration is limited to the data protection keychain](https://developer.apple.com/forums/thread/719342)
* [Agents and daemons reference](https://developer.apple.com/library/archive/technotes/tn2083/_index.html) – see especially [Execution contexts](https://developer.apple.com/library/archive/technotes/tn2083/_index.html#//apple_ref/doc/uid/DTS10003794-CH1-SECTION9)
* [Bootstrap contexts reference](https://developer.apple.com/library/archive/documentation/Darwin/Conceptual/KernelProgramming/contexts/contexts.html)
* [Demystifying the Secure Enclave Processor](https://www.blackhat.com/docs/us-16/materials/us-16-Mandt-Demystifying-The-Secure-Enclave-Processor.pdf) – see mostly Communication, starting slide 41, and possibly Entitlements, starting slide 76
* [Mac keychains reference](https://developer.apple.com/documentation/technotes/tn3137-on-mac-keychains)

## Troubleshooting and testing notes

Mostly I used a lot of `log show` with timestamps corresponding to the timestamps of error logs from launcher, or `log stream` while unloading and reloading launcher. You can filter by the predicate `--predicate 'process == "launcher"'` but then you do miss some context around authentication.
