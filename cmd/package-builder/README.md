# Building Packages

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
./build/package-builder make --hostname=grpc.launcher.example.com:443 --enroll_secret=foobar123
```

If you'd like to customize the keys that are used to sign the enrollment secret and macOS package, consider the following usage:

```
./build/package-builder make \
  --hostname=localhost:8082 \
  --enroll_secret=foobar123 \
  --osquery_version=stable \
  --mac_package_signing_key="Developer ID Installer: Example Inc (ABCDEF123456)"
```

The macOS package will install a LaunchDaemon that will connect the launcher to the server specified by the `--hostname` flag, using an enrollment secret specified by the `--enroll_secret` flag. The Linux packages will currently lay down the launcher and osquery binaries as well as the enrollment secret specified by the `--enroll_secret` flag.

If you would like the resultant launcher binary to be invoked with any of the following flags, include them with the invocation of `package-builder`:

- `--insecure`
- `--insecure_grpc`
- `--autoupdate`
- `--update_channel`

For example, consider the following usage:

```
./build/package-builder make \
  --hostname=localhost:8082 \
  --enroll_secret=foobar123 \
  --insecure \
  --insecure_grpc \
  --autoupdate \
  --update_channel=nightly
```

By default, binaries will be installed to `/usr/local/launcher/bin`, configuration will be installed to `/etc/launcher`, logs will be outputted to `/var/log/launcher`, etc. If you'd like the `launcher` string to be something else (for example, your company name), you can use the `--identifier` flag to specify this value. If you would like the resultant packages to not contain the enroll secret (so that you can distribute it via another mechanism), you can use the `--omit_secret` flag.
