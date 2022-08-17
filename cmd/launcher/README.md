# The Osquery Launcher

## Building the tool

From the root of the repository, run the following:

```
make deps
make launcher
./build/launcher --help
```

To install the launcher binaries to `$GOPATH/bin`, run the following:

```
make deps
make install
```

You *could* run `go get github.com/kolide/launcher/cmd/...` to install the binaries but it is not recommended because the binaries will not be built with version information when you run `launcher --version`.

## General Usage

To use The Launcher to easily connect osquery to a server that is compliant with the [gRPC specification](https://github.com/kolide/agent-api/blob/main/agent_api.proto), invoke the binary with just a few flags:

- `--hostname`: the hostname of the gRPC server for your environment
- `--root_directory`: the location of the local database, pidfiles, etc.
- `--enroll_secret`: the enroll secret that is used in your environment
- `--autoupdate`: a boolean flag which controls the osqueryd autoupdater (default: true)

```
./build/launcher \
  --hostname=fleet.example.com:443 \
  --root_directory=/var/kolide-fleet \
  --enroll_secret=32IeN3QLgckHUmMD3iW40kyLdNJcGzP5
```

You can also define the enroll secret via a file path
(`--enroll_secret_path`) or an environment variable
(`KOLIDE_LAUNCHER_ENROLL_SECRET`). See `launcher --help` for more
information.

Depending on your transport configuration, you may need any of the
`--transport`, `--insecure` or `--insecure_transport` flags.

### Running an extension socket

To run a launcher-powered extension socket, run `launcher socket` and the path of the socket will be printed to stdout:

```
./build/launcher socket
/var/folders/wp/6fkmvjf11gv18tdprv4g2mk40000gn/T/osquery.sock
^C
exiting...
```

To run the socket at a defined path, use the `--path` flag:

```
./build/launcher socket --path=/tmp/sock
/tmp/sock
```

### Querying an extension socket

To run queries against an existing extension socket, use `launcher query`. You must define the socket path via the `--socket` flag. Query JSON can be provided via stdin or a file specified via the `--queries` flag. Consider an example querying the socket via queries defined in a file:

```
$ cat queries.json
{
  "queries": {
    "apps": "select name, path from apps limit 2",
    "hostname": "select hostname from system_info"
  }
}
$ ./build/launcher query --socket=/tmp/osquery.sock --queries=./queries.json
{
    "results": {
        "apps": [
            {
                "name": "1Password 6.app",
                "path": "/Applications/1Password 6.app"
            },
            {
                "name": "2BUA8C4S2C.com.agilebits.onepassword4-helper.app",
                "path": "/Applications/1Password 6.app/Contents/Library/LoginItems/2BUA8C4S2C.com.agilebits.onepassword4-helper.app"
            }
        ],
        "hostname": [
            {
                "hostname": "marpaia"
            }
        ]
    }
}
```

Now consider an example using stdin:

```
$ cat queries.json | ./build/launcher query --socket=/tmp/osquery.sock
{
    "results": {
        "apps": [
            {
                "name": "1Password 6.app",
                "path": "/Applications/1Password 6.app"
            },
            {
                "name": "2BUA8C4S2C.com.agilebits.onepassword4-helper.app",
                "path": "/Applications/1Password 6.app/Contents/Library/LoginItems/2BUA8C4S2C.com.agilebits.onepassword4-helper.app"
            }
        ],
        "hostname": [
            {
                "hostname": "marpaia"
            }
        ]
    }
}
```

## Examples

### Connecting to Fleet

Let's say that you have [Kolide Fleet](https://github.com/kolide/fleet) running at https://fleet.example.com, you could simply run the following to connect The Launcher to Fleet (assuming you replace the enroll secret with the correct string):

```
launcher \
  --enroll_secret=32IeN3QLgckHUmMD3iW40kyLdNJcGzP5 \
  --hostname=fleet.example.com:443 \
  --root_directory=/var/launcher/fleet
```

If you're running Fleet on the default development location (https://localhost:8080), you can connect a launcher via:

```
mkdir /tmp/fleet-launcher
launcher \
  --enroll_secret=32IeN3QLgckHUmMD3iW40kyLdNJcGzP5 \
  --hostname=fleet.example.com:443 \
  --root_directory=/tmp/fleet-launcher \
  --insecure
```

Note the `--insecure` flag.
