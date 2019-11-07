# Upgrade Exec Service Testing

This is a test program to explore update functionality, primarily
focuses on windows.It pairs with the [Autoupdate ADR](/
docs/architecture/2019-03-11_autoupdate.md)

## Observations

You cannot remove an exe that's linked to a _running_ service. You can
rename it. You can't overwrite it. This manifests as a permission
error. If the service is loaded, but stopped, you can overwrite.

Note that if you're using the restart-on-failure, this can create a
small race condition. If the service manager attempts a restart while
there is _no_ binary on the expected path, the service transitions to
a stopped state. This timing is effected by the restart delay
settings.

### Windows Services Restart On Failure

Windows Service Manager can restart services on failure. This is
controlled by the Recovery portion of the settings. Behavior seems the
same regardless of exit code. Process _does_ get a new PID on launch

These configs can be examined using the `sc.exe` tool. (Note: you
might need an extra param to adjust the buffer size.)

For example:

``` shell
PS C:\Users\example> sc.exe qfailure upgradetest 5000
[SC] QueryServiceConfig2 SUCCESS

SERVICE_NAME: upgradetest
        RESET_PERIOD (in seconds)    : 0
        REBOOT_MESSAGE               :
        COMMAND_LINE                 :
```

Further Reading:

* [WiX ServiceConfig](http://wixtoolset.org/documentation/manual/v3/xsd/util/serviceconfig.html)
* [go configs](https://godoc.org/golang.org/x/sys/windows/svc/mgr#Service.RecoveryActions)
* [StackExchange](https://serverfault.com/questions/48600/how-can-i-automatically-restart-a-windows-service-if-it-crashes)

**Note:** The MSI options to configure this are broken. The
recommendation is to use a Custom Action to call out to
`sc.exe`. Instead, we handle inside the service start. 

### Replace before restart

Manually testing the idea of moving a binary aside, and dropping in a
new one and then calling `Exit(0)` and letting the service manager
restart... This seems to work

Test Process:
1. Use my test case of run 5s, exit.
2. svc manager restarts
3. Observe new PIDs.
4. During a 5s loop, move old binary aside and scp new binary in
5. Observe the format of the log messages change on restart

## Shell Debugging Snippets

Viewing Event Log:
```
# Old interface
Get-EventLog -LogName Application -Newest 10

# New interface, with full bodies
Get-WinEvent -LogName System -MaxEvents 10
Get-WinEvent -LogName Application -MaxEvents 10 | Format-Table TimeCreated,Message -wrap
```


Various ways to see service status:
``` shell
Get-Service upgradetest

sc.exe query upgradetest
sc.exe qc  upgradetest
sc.exe qfailure upgradetest 5000
```
