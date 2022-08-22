# The Osquery Launcher

## Building the Code

Requirements:
* Repository checkout (Not dependent on `$GOPATH`)
* Recent go (currently depends on 1.16)
* [`zig`](https://ziglang.org/) compiler, if and only if, cross compiling for linux

Then, from your checkout, run:

```
make deps
make launcher
./build/launcher --help
```

Note that this style of build is generally only for development
instances of Launcher. You should have `osqueryd` already installed on
your system, as `launcher` will fall-back to looking for it in your
`$PATH` in this case. For additional, more distributable, build
options and commands, see the [Additional Build
Options](#additional-build-options) section.

For more distributable packages of Launcher and Osquery, consider
using the [`package-builder`](./package-builder.md) tool that is
provided with this repository.

## General Usage

To use The Launcher to easily connect osquery to a server that is
compliant with the [gRPC
specification](https://github.com/kolide/agent-api/blob/main/agent_api.proto),
invoke the binary with just a few flags:

- `--hostname`: the hostname of the gRPC server for your environment
- `--root_directory`: the location of the local database, pidfiles, etc.
- `--enroll_secret`: the enroll secret that is used in your environment
- `--autoupdate`: a boolean flag which controls the osqueryd autoupdater (default: false)

```
./build/launcher \
  --hostname=fleet.example.net:443 \
  --root_directory=/var/kolide-fleet \
  --enroll_secret=32IeN3QLgckHUmMD3iW40kyLdNJcGzP5
```

You may need to define the `--insecure` and/or `--insecure_grpc` flag
depending on your server configurations.

## Configuring Launcher

Launcher supports runtime configuration via three mechanisms. It reads
environmental variables, then a config file, and lastly command line
flags. As an example, to set the hostname, you would do one of:

1. set the environmental variable `KOLIDE_LAUNCHER_HOSTNAME=fleet.example.net:443`
1. Include `hostname fleet.example.net:443` in a config.flags file
1. invoke with `--hostname=fleet.example.net:443` on the command line

While any of these can be used in conjunction with your init system,
we generally recommend using config files for simplicity. As a side
note, environmental variables on windows are global, and thus
contraindicated for configuration data.


## Override Osquery Flags

In some scenarios, users may wish to specify additional flags for
osquery, or override the default values set by Launcher. To do this,
use `--osquery_flag`. This option may be specified more than once to
set multiple flags:

```
./build/launcher \
  --hostname=fleet.acme.net:443 \
  --osquery_flag windows_event_channels=foo,bar
  --osquery_flag logger_plugin=filesystem
```

Any flags specified in this manner will be passed at the end of the
osquery command. They will take precedence over any other flags set.

Note that it is entirely possible to break Launcher's expected
functionality using this option. **Be careful when overriding flags!**

Because of likely breakage, the following flags cannot be overridden:

- `--pidfile`
- `--database_path`
- `--extensions_socket`
- `--extensions_autoload`
- `--extensions_timeout`
- `--config_plugin`

## Examples

### Connecting to Fleet

Let's say that you have [Kolide
Fleet](https://github.com/kolide/fleet) running at
https://fleet.example.org, you could simply run the following to
connect The Launcher to Fleet (assuming you replace the enroll secret
with the correct string):

```
launcher \
  --enroll_secret=32IeN3QLgckHUmMD3iW40kyLdNJcGzP5 \
  --hostname=fleet.example.org:443 \
  --root_directory=/var/launcher/fleet
```

If you're running Fleet on the default development location
(https://localhost:8080), you can connect a launcher via:

```
mkdir /tmp/fleet-launcher
launcher \
  --enroll_secret=32IeN3QLgckHUmMD3iW40kyLdNJcGzP5 \
  --hostname=fleet.example.org:443 \
  --root_directory=/tmp/fleet-launcher \
  --insecure
```

Note the `--insecure` flag.

### Certificate Pinning

Launcher supports pinning to the `SubjectPublicKeyInfo` of
certificates in the verified chain. To enable this feature, provide
the SHA256 hashes of the `SubjectPublicKeyInfo` that should be pinned
as comma-separated hex-encoded values to the `cert_pins` flag.

For example, to pin to `intermediate.crt` we could do the following:

```
openssl x509 -in intermediate.crt -pubkey -noout | openssl pkey -pubin -outform der | openssl dgst -sha256
b48364002b8ac4dd3794d41c204a0282f8cd4f7dc80b26274659512c9619ac1b

launcher --cert_pins=b48364002b8ac4dd3794d41c204a0282f8cd4f7dc80b26274659512c9619ac1b
```

### Specify Root CAs

If your server TLS certificate is signed by a root that is not
recognized by the system trust store, you will need to manually point
launcher at the appropriate root to use. Note, if you specify any
roots with this method, _only_ those roots will be used, and the
system store will be ignored.

The PEM file should contain all of the root authorities you would like
launcher to be able to validate against.


```
launcher --root_pem=root.pem
```
## Running Launcher with systemd
See [systemd](./systemd.md) for documentation on running launcher as a
background process.


## Additional Build Options

### Normal development build

From the root of the repository, run the following:

```
make deps
make launcher
./build/launcher --help
```

### Cross-compiling binaries

To build macOS and Linux binaries, run the following:

```
make deps
make xp
```

Consider the following paths and file information:

```
$ file build/linux/launcher
build/linux/launcher: ELF 64-bit LSB executable, x86-64, version 1 (SYSV), statically linked, with debug_info, not stripped
$ file build/darwin/launcher
build/darwin/launcher: Mach-O 64-bit executable x86_64
```

### Generating relocatable binary bundles

```
make deps
make binary-bundle
```

This will result in two files:

- `build/binary-bundle/launcher_0.4.0.zip`
- `build/binary-bundle/launcher_latest.zip`

Each zip will contain the following files:

```
|-- darwin
|   |-- launcher
|   `-- osqueryd
`-- linux
    |-- launcher
    `-- osqueryd
```
