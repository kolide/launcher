# Building Packages

## Background & Requirements

Right now, `package-builder` will build a macOS package (.pkg), a debian package (.deb), and an RPM (.rpm). The macOS package is built and signed using the `pkgbuild` command, which is only found on macOS. [There are plans](https://github.com/kolide/launcher/issues/188) to allow for the building of mac packages on Linux but until then, it is a requirement to use a macOS machine when running the `package-builder` tool.

To build the Linux packages, `package-builder` will execute a docker container which contains the [`fpm`](https://github.com/jordansissel/fpm) tool. Thus, you must also have docker installed and working correctly. If you would like to test this, run the following:

```
docker run --rm -it kolide/fpm echo "it works"
```

Finally, like any Go project, you must have this repo checked out to `$GOPATH/src/github.com/kolide/launcher`. If you're new to Go and you don't know about `$GOPATH`, then check out the repo to `$HOME/go/src/github.com/kolide/launcher`. Also, verify that `$GOPATH/bin` is listed in your `$PATH` (it may appear there as `$HOME/go/bin`).

So to recap, to use `package-builder`, you must:

- Be on macOS
- Be able to `docker run` something
- Build the tool from source

## Building the tool

From the root of the repository, run the following:

```
make deps
make package-builder
./build/package-builder --help
```

## General Usage

### Creating a set of packages

Assuming you have built the `package-builder` tool and the `launcher` binaries via `make package-builder`, you can create a set of launcher packages by using the `package-builder make` command. The only required parameter is `--hostname`. If you don't define an enrollment secret via `--enroll_secret`, then a blank enrollment secret will be used when connecting to the gRPC server defined by the supplied hostname.

```
./build/package-builder make --hostname=grpc.launcher.acme.biz:443 --enroll_secret=foobar123
```

If you'd like to customize the keys that are used to sign the enrollment secret and macOS package, consider the following usage:

```
./build/package-builder make \
  --hostname=localhost:8082 \
  --enroll_secret=foobar123 \
  --osquery_version=stable \
  --mac_package_signing_key="Developer ID Installer: Acme Inc (ABCDEF123456)"
```

The macOS package will install a LaunchDaemon that will connect the launcher to the server specified by the `--hostname` flag, using an enrollment secret specified by the `--enroll_secret` flag. The Linux packages will currently lay down the launcher and osquery binaries as well as the enrollment secret specified by the `--enroll_secret` flag.

If you would like the resultant launcher binary to be invoked with any of the following flags, include them with the invocation of `package-builder`:

- `--insecure`
- `--insecure_grpc`
- `--autoupdate`
- `--update_channel`
- `--cert_pins`

For example, consider the following usage:

```
./build/package-builder make \
  --hostname=localhost:8082 \
  --enroll_secret=foobar123 \
  --insecure \
  --insecure_grpc \
  --autoupdate \
  --update_channel=nightly \
  --cert_pins=5dc4d2318f1ffabb80d94ad67a6f05ab9f77591ffc131498ed03eef3b5075281
```

By default, binaries will be installed to `/usr/local/launcher/bin`, configuration will be installed to `/etc/launcher`, logs will be outputted to `/var/log/launcher`, etc. If you'd like the `launcher` string to be something else (for example, your company name), you can use the `--identifier` flag to specify this value. If you would like the resultant packages to not contain the enroll secret (so that you can distribute it via another mechanism), you can use the `--omit_secret` flag.
