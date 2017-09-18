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

To use The Launcher to easily connect osquery to a server that is compliant with the [gRPC specification](https://github.com/kolide/agent-api/blob/master/agent_api.proto), invoke the binary with just a few flags:

- `--hostname`: the hostname of the gRPC server for your environment
- `--root_directory`: the location of the local database, pidfiles, etc.
- `--enroll_secret`: the enroll secret that is used in your environment

```
./build/launcher \
  --hostname=fleet.acme.net:443 \
  --root_directory=/var/kolide-fleet \
  --enroll_secret=32IeN3QLgckHUmMD3iW40kyLdNJcGzP5
```

You can also define the enroll secret via a file path (`--enroll_secret_path`) or an environment variable (`KOLIDE_LAUNCHER_ENROLL_SECRET`). See `launcher --help` for more information.

You may need to define the `--insecure` and/or `--insecure_grpc` flag depending on your server configurations.

## Examples

### Connecting to Fleet

Let's say that you have [Kolide Fleet](https://github.com/kolide/fleet) running at https://fleet.acme.org, you could simply run the following to connect The Launcher to Fleet (assuming you replace the enroll secret with the correct string):

```
launcher \
  --enroll_secret=32IeN3QLgckHUmMD3iW40kyLdNJcGzP5 \
  --hostname=fleet.acme.org:443 \
  --root_directory=/var/acme/fleet
```

If you're running Fleet on the default development location (https://localhost:8080), you can connect a launcher via:

```
mkdir /tmp/fleet-launcher
launcher \
  --enroll_secret=32IeN3QLgckHUmMD3iW40kyLdNJcGzP5 \
  --hostname=fleet.acme.org:443 \
  --root_directory=/tmp/fleet-launcher \
  --insecure
```

Note the `--insecure` flag.
