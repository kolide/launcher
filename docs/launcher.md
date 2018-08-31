# The Osquery Launcher

## Building the Code

Checkout this repository to `$GOPATH/src/github.com/kolide/launcher`. If you're new to Go and you don't know about `$GOPATH`, then check out the repo to `$HOME/go/src/github.com/kolide/launcher`. You will also need to install Go (1.9 or greater).

From the root of the repository, run the following:

```
make deps
make launcher
./build/launcher --help
```

Note that this style of build is generally only for development instances of Launcher. You should have `osqueryd` already installed on your system, as `launcher` will fall-back to looking for it in your `$PATH` in this case. For additional, more distributable, build options and commands, see the [Additional Build Options](#additional-build-options) section.

For more distributable packages of Launcher and Osquery, consider using the [`package-builder`](./package-builder.md) tool that is provided with this repository.

## General Usage

To use The Launcher to easily connect osquery to a server that is compliant with the [gRPC specification](https://github.com/kolide/agent-api/blob/master/agent_api.proto), invoke the binary with just a few flags:

- `--hostname`: the hostname of the gRPC server for your environment
- `--root_directory`: the location of the local database, pidfiles, etc.
- `--enroll_secret`: the enroll secret that is used in your environment
- `--autoupdate`: a boolean flag which controls the osqueryd autoupdater (default: true)

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

### Certificate Pinning

Launcher supports pinning to the `SubjectPublicKeyInfo` of certificates in the verified chain. To enable this feature, provide the SHA256 hashes of the `SubjectPublicKeyInfo` that should be pinned as comma-separated hex-encoded values to the `cert_pins` flag.

For example, to pin to `intermediate.crt` we could do the following:

```
openssl x509 -in intermediate.crt -pubkey -noout | openssl pkey -pubin -outform der | openssl dgst -sha256
b48364002b8ac4dd3794d41c204a0282f8cd4f7dc80b26274659512c9619ac1b

launcher --cert_pins=b48364002b8ac4dd3794d41c204a0282f8cd4f7dc80b26274659512c9619ac1b
```

### Specify Root CAs

If your server TLS certificate is signed by a root that is not recognized by the system trust store, you will need to manually point launcher at the appropriate root to use. Note, if you specify any roots with this method, _only_ those roots will be used, and the system store will be ignored.

The PEM file should contain all of the root authorities you would like launcher to be able to validate against.


```
launcher --root_pem=root.pem
```
## Running Launcher with systemd
See [systemd](./systemd.md) for documentation on running launcher as a background process.


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
|   |-- osquery-extension.ext
|   `-- osqueryd
`-- linux
    |-- launcher
    |-- osquery-extension.ext
    `-- osqueryd
```

### Installing the binaries for local usage

To install the launcher binaries to `$GOPATH/bin`, run the following:

```
make deps
make install
```

You *could* run `go get github.com/kolide/launcher/cmd/...` to install the binaries but it is not recommended because the binaries will not be built with version information when you run `launcher --version`.
