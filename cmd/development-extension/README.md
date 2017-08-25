Querying launcher tables in osqueryi
====================================

The Launcher includes a few custom tables. In order to test these tables with `osqueryi`, an extension is provided that allows you to attach all launcher provided tables to an `osqueryi` instance. To do this, the command you should use is `make osqueryi`:

```
$ make osqueryi
go build -i -o build/development-extension.ext ./cmd/development-extension/
osqueryi --extension=./build/development-extension.ext
Using a virtual database. Need help, type '.help'
osquery> select * from personal_artifacts;
+-------+----------------+
| type  | value          |
+-------+----------------+
| email | mike@kolide.co |
| email | mike@arpaia.co |
+-------+----------------+
osquery>
```

As you can see from the above example, `make osqueryi` compiles an extension and then launches `osqueryi` with the resultant extension. All launcher provided tables will now be available.
