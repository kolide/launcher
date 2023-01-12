# Methods chosen for sending notifications to the end user.

## Status

Implemented, but not yet available to end users on the stable release channel.

## Context

Kolide, via the launcher application, would like to alert end users of potential issues
on their devices that they may want to remediate. Kolide will send notifications
to launcher via the control server; launcher must then display those notifications to
the end user in a way that makes it clear to the end user that Kolide is legitimately
sending them a notification.

## Decision

### Methods implemented

For MacOS: we use the objective-c API to [request authorization to send notifications](https://developer.apple.com/documentation/usernotifications/unusernotificationcenter/1649527-requestauthorizationwithoptions?language=objc)
and then to [send them](https://developer.apple.com/documentation/usernotifications/unusernotificationcenter/1649508-addnotificationrequest).

For Windows: we use [go-toast](https://pkg.go.dev/gopkg.in/toast.v1) -- under the hood,
this writes a temporary powershell script that loads XML defining the notification, and
then sends it.

For Linux: we send via [dbus's notification service](https://specifications.freedesktop.org/notification-spec/notification-spec-latest.html)
if available; we fall back to sending via [notify-send](https://ss64.com/bash/notify-send.html)
if not.

Note that these methods required, at least for Windows and Linux, that the [notification
service](../../ee/desktop/notify) run in the user process, rather than the parent launcher
process.

Additionally note that both the Windows and Linux implementation options required a filepath
to the icon file, so we write out that file to disk. The MacOS implementation automatically
uses the icon file shipped inside the app bundle.

### Methods that were considered but discarded

[fyne](https://pkg.go.dev/fyne.io/fyne) (both 1.4 and 2.0) -- worked well for macOS, but it
would have been non-trivial to get the graphics libraries linking properly for Windows and
Linux, and it didn't seem worth it at this time to invest that effort when we don't require
those libraries. If we decide to invest the effort in the future, 2.0 would be preferable
over 1.4 because 1.4 is using deprecated macOS APIs.

[zenity](https://pkg.go.dev/github.com/ncruces/zenity) -- on Windows, notifications displayed
with a header `Microsoft.Explorer.Notification.{UUID}` that didn't appear to be configurable
and would probably confuse end users; on Linux, it would not display the title.

A native solution for Windows -- seems like it should be possible but I couldn't figure it out.
In the future, we may want to research this option further.

[kdialog](https://develop.kde.org/deploy/kdialog/#--passivepopup-dialog-box) -- the header for
the popup always says `kdialog`, which is not ideal.

[osascript display notification](https://developer.apple.com/library/archive/documentation/LanguagesUtilities/Conceptual/MacAutomationScriptingGuide/DisplayNotifications.html) --
not possible to configure the icon; additionally, when the end user clicks the notification,
it opens up `Script Editor`, which is a confusing UX. (Some docs indicated that running the
script from within your bundle should force it to use the bundle's icon, but people noted
that this functionality seems broken and it did not work for me in test.) I originally had
this as a fallback option for macOS because that's what some other libraries do, but
ultimately decided it was not ideal enough that it wasn't worth keeping.

## Consequences

We are now able to send notifications on all OSes to end users. We may find that the current
options do not support future improvements that we may want to make, such as custom icons
per-notification, or action buttons.
