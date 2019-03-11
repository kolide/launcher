## Observations

You cannot remove an exe that's linked to a _running_ service. You can
rename it. You can't overwrite it. This manifests as a permission
error. If the service is loaded, but stopped, you can however,
overwrite.

Note that if you're using the restart-on-failure, this can create a
small race condition. If the service manager attempts a restart while
there is _no_ binary on the expected path, the service transitions to
a stopped state. (This is probably the window of time in the restart delay)

### Windows Services Restart On Failure

Windows Service Manager can restart services on failure. This is
controlled by the Recovery portion of the settings. Behavior seems the
same regardless of exit code. Process _does_ get a new PID on launch

These configs can be examined using the `sc.exe` tool. (Note: you
might need an extra param to adjust the buffer size. I don't even)

For example: 
``` shell
PS C:\Users\seph> sc.exe qfailure sephexec 5000
[SC] QueryServiceConfig2 SUCCESS

SERVICE_NAME: sephexec
        RESET_PERIOD (in seconds)    : 0
        REBOOT_MESSAGE               :
        COMMAND_LINE                 :
```

Further Reading:

* [WiX ServiceConfig](http://wixtoolset.org/documentation/manual/v3/xsd/util/serviceconfig.html)
* [go configs](https://godoc.org/golang.org/x/sys/windows/svc/mgr#Service.RecoveryActions)
* [StackExchange](https://serverfault.com/questions/48600/how-can-i-automatically-restart-a-windows-service-if-it-crashes)

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

## Snippets


```

Get-EventLog -LogName Application -Newest 10

Get-WinEvent -LogName System -MaxEvents 10
Get-WinEvent -LogName Application -MaxEvents 10

| Format-Table TimeCreated,Message -wrap
```


``` shell
Get-Service sephexec
Start-Service sephexec

sc.exe query sephexec
sc.exe qc  sephexec
sc.exe qfailure sephexec 5000
```



Get-WinEvent -FilterHashTable @{Logname='System';ID=3396} | 
 
Get-WinEvent -LogName System 
