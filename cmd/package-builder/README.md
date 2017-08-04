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

If you would like the resultant launcher binary to be invoked with the `--insecure` or `--insecure_grpc` flags, include them with the invocation of `package-builder`:

```
./build/package-builder make \
  --hostname=localhost:8082 \
  --enroll_secret=foobar123 \
  --insecure \
  --insecure_grpc
```

## Kolide Usage

### Authentication

You must be authenticated to Kolide's GCloud organization for the various `package-builder` commands to work. To do this, you will need to install the GCloud tools. Documentation on using and installing these tools can be found [here](https://cloud.google.com/sdk/gcloud/).

To authenticate to GCloud, use the following:

```
gcloud auth application-default login
```
#### Notary Setup
Notary Client must be properly installed and **be in your search path** in order to publish binaries. Notary Client can be found [here](https://github.com/docker/notary).  Prepare
Notary Client as follows.
```
git clone ssh://git@github.com/docker/notary.git
cd notary
make client
```
Download `notary.zip` from Kolide 1Password and extract it into your home directory.
```
mv notary.zip ~/.
cd
unzip notary.zip
sudo echo '35.196.246.167 notary.kolide.com' >> /etc/hosts
```

### Development Packages

To use the tool to generate Kolide internal development packages, run:

```
./build/package-builder dev --debug \
  --mac_package_signing_key="Developer ID Installer: Acme Inc (ABCDEF123456)"
```

### Production Packages

To use the tool to generate Kolide production packages, run:

```
./build/package-builder prod --debug \
  --enrollment_secret_signing_key=/path/to/key.pem \
  --mac_package_signing_key="Developer ID Installer: Acme Inc (ABCDEF123456)"
```

### Mirror Command

Osquery and Launcher binaries must be available on a remote mirror so the autoupdate can download them. The binaries also need to be available when building a new platform specific package.
Binaries are packaged in a tarball in the format `<binary-name>-<update-channel or version>.tar.gz`
Example:
    ```
    osqueryd-stable.tar.gz
    osqueryd-2.6.0.tar.gz
    ```
The tarballs are stored in the `gs://binaries-for-launcher` GCS bucket, and exposed at the `https://dl.kolide.com/kolide/<binary>/<platform>/<tarball>` url in the `kolide-website` GCP project.

The `mirror` subcommand may be used to produce tar archives for both Launcher and Osquery, upload them
to the mirror site, and register them with Notary so that they can be validated as part of the Launcher autoupdate process.  In order to publish releases to Notary using the mirror command the following environment variables should be defined using passwords for respective Notary keys.
```
NOTARY_SNAPSHOT_PASSPHRASE=<snapshot secret>
NOTARY_TARGETS_PASSPHRASE=<targets secret>
```
The default `mirror` flag is `-all` which will perform all the steps needed to release new binaries. If `-all=false` individual subcommands can be used to perform a subset of these operations.  For example, you could choose to publish changes for Osquery or only create tarballs for Launcher.  The following commands would download and
publish the latest version of Osquery, and would publish version 1.2 of Launcher to the `stable` channel for the `darwin` platform.
```
$ git tag 1.2
$ gcloud config set project kolide-website
$ make package-builder
$ build/package-builder mirror
```
Verify packages were registered with Notary.
```
$ notary list kolide/launcher

NAME                             DIGEST                                                              SIZE (BYTES)    ROLE
----                             ------                                                              ------------    ----
darwin/launcher-1.2.tar.gz       7504970563e879344ae7a4264e3e13d72dfdb7acab155be18d4971252755326e    4805095         targets
darwin/launcher-stable.tar.gz    7504970563e879344ae7a4264e3e13d72dfdb7acab155be18d4971252755326e    4805095         targets
```
Help is available from the command line.

```
$ build/package-builder mirror -help
```


### Version info

```
./build/package-builder prod --debug \
  --enroll_secret_signing_key=/path/to/key.pem \
  --mac_package_signing_key="Developer ID Installer: Acme Inc (ABCDEF123456)"
```
