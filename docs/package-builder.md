# Building Packages

## Background & Requirements

Kolide launcher packages are a collection of binaries (`osqueryd`,
`launcher`, and `osquery-extension.ext`), configuration, and init
scripts. This repository contains `package-builder`, a tool to produce
these packages.

`package-builder` is written in go, and should run from most
platforms. It is most commonly run on macOS and Windows. It can be
cross compiled if desired.

### Building macOS packages

To produce macOS packages, you must be running on macOS.

The macOS package is built and signed using the `pkgbuild` command,
which is only found on macOS. [There are
plans](https://github.com/kolide/launcher/issues/188) to allow for the
building of mac packages on Linux but until then, it is a requirement
to use a macOS machine when running the `package-builder` tool.

### Building linux packages

To build the Linux packages, `package-builder` will execute a docker
container which contains the
[`fpm`](https://github.com/jordansissel/fpm) tool. Thus, you must also
have docker installed and working correctly. If you would like to test
this, run the following:

```
docker run --rm -it kolide/fpm echo "it works"
```

### Building windows packages

Windows packages use `wix` and `package-builder` must be run on a
windows machine with binaries in the appropriate places.


## Building the tool

From the root of the repository, run the following:

``` shell
make package-builder
./build/package-builder make --help
```

If you want to cross compile for windows, you can use:

``` shell
make package-builder-windows
ls build/windows/package-builder.exe
```

## General Usage

`package-builder` will fetch binaries from either local disk, or
Kolide's Notary server. These are specified with version command line
options. Arguments that look like a path (denoted by starting with `/`
or `./`) will be pulled from local disk, otherwise the argument is
parsed as a notary channel.

The only required parameter is `--hostname`.

If you don't define an enrollment secret via `--enroll_secret`, then a
blank enrollment secret will be used when connecting to the server
defined by the supplied hostname. If you would like the resultant
packages to not contain the enroll secret (so that you can distribute
it via another mechanism), you can use the `--omit_secret` flag.

### Simplest Package Creation

The simplest usage, is to use the binaries on the Kolide Notary
server:

``` shell
./build/package-builder make \
   --hostname=grpc.launcher.example.com:443 \
   --enroll_secret=foobar123
```


### Building a package with your own binaries


To build launcher binaries you'll want something like:

```
# Build unsigned launcher binaries
make -j xp
```

Or to build signed:

```
# Replace this with your codesign information
export CODESIGN_IDENTITY='Developer ID Application: Acme Inc (ABCDEF123456)'
make xp-codesign
```

You can now use `package-builder` to make packages with those:

```
./build/package-builder make \
   --hostname=grpc.launcher.example.com:443 \
   --enroll_secret=foobar123 \
   --osquery_version stable \
   --launcher_version ./build/darwin/launcher \
   --extension_version ./build/darwin/osquery-extension.ext
```

If you'd like to customize the keys that are used to sign the
enrollment secret and macOS package, consider adding the
`--mac_package_signing_key` option.


If you would like the resultant launcher binary to be invoked with any
of the following flags, include them with the invocation of
`package-builder`:

- `--insecure`
- `--insecure_grpc`
- `--autoupdate`
- `--update_channel`
- `--cert_pins`

### Override Osquery Flags

[Osquery override flags](./launcher.md#override-osquery-flags) can be built into packages made with `package-builder`. Use the `--osquery_flag` option. This option may be specified more than once to set multiple flags:

```
./build/package-builder make \
  --hostname=fleet.acme.net:443 \
  --osquery_flag windows_event_channels=foo,bar
  --osquery_flag logger_plugin=filesystem
```

Any flags specified in this manner will be passed at the end of the osquery command. They will take precedence over any other flags set.

### Caveats

#### Identifiers

By default, binaries will be installed to `/usr/local/launcher/bin`,
configuration will be installed to `/etc/launcher`, logs will be
outputted to `/var/log/launcher`, etc. If you'd like the `launcher`
string to be something else (for example, your company name), you can
use the `--identifier` flag to specify this value.

#### Cross Platform Binaries and Targets

`package-builder` can package cross platform. If you're obtaining
binaries from notary, this should be straight forward, and you can
specify multiple targets in a single invocation.  However, if you're
using locally build binaries you will need to run `package-builder`
for each target platform.

Targets can be specificity as a _platform-init-packaging_ triple.

To display the list of support targets, invoke `package-builder list-targets`

#### Docker Temp Directories

Packaging for linux used `fpm` via a docker container. This operates
on files in a directory created by golang's `ioutil.TempFile`. On osx,
the default temp directory is in `/var`, which is not accessible to
docker.

This will manifest as an error along the lines of:

``` shell
could not generate packages: making package: packaging, target linux-systemd-rpm: creating fpm package: docker: Error response from daemon: Mounts denied:
The paths /var/folders/jj/ypss_3r13d374nz95cdw418r0000gn/T/package.scriptRoot916184548 and /var/folders/jj/ypss_3r13d374nz95cdw418r0000gn/T/package.packageRoot332969561
are not shared from OS X and are not known to Docker.
You can configure shared paths from Docker -> Preferences... -> File Sharing.
See https://docs.docker.com/docker-for-mac/osxfs/#namespaces for more info.
```

As `ioutil.TempFile` respects the `TMPDIR` environmental variable, there is a simple workaround:

``` shell
export TMPDIR=/tmp
```

#### Windows

Windows can be built without a service `windows-none-msi` or with a
service `windows-service-msi`.

Note that the windows package will only install as `ALLUSERS`. You may
need to use elevated privileges to install it. This will likely be
confusing. `msiexec.exe` will either silently fail, or be
inscrutable. But `start` will work.
