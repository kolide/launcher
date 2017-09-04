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

### Publishing Updates

#### Configuration

Notary Client must be properly installed and **be in your search path** in order to publish binaries. Notary Client can be found [here](https://github.com/docker/notary).  Prepare Notary Client as follows.

```
git clone ssh://git@github.com/docker/notary.git $GOPATH/src/github.com/docker/notary
cd $GOPATH/src/github.com/docker/notary
make client
```
Configure Notary as follows. Download the internal `notary.zip` from Kolide and extract it into your home directory.
```
unzip ./notary.zip -d ~/
```
#### Initial Repository Setup
Create environment variables with strong pass phrases to use to encrypt notary keys.
```
NOTARY_DELEGATION_PASSPHRASE=<secret>
NOTARY_ROOT_PASSPHRASE=<secret>
NOTARY_SNAPSHOT_PASSPHRASE=<secret>
NOTARY_TARGETS_PASSPHRASE=<secret>
```
Define GUNs (Global Unique Identifiers) for two repsitories, one for Launcher and one for Osqueryd.
```
notary init kolide/launcher -p
notary init kolide/osqueryd -p
```
Find the ID of the root key. This key will be created automatically when the first repository
is created.
```
notary key list

ROLE        GUN                KEY ID                                                              LOCATION
----        ---                ------                                                              --------
root                           b8dc5cded1a8522a563a58c3ac7ad2eba51d6945999aa5864678fb5064bb6f9e    /Users/jam/.notary/private
snapshot    kolide/launcher    5e1221edd379be729f12f3cb69786758ee23a71067b6e25c62d10ccfe0c82f31    /Users/jam/.notary/private
targets     kolide/launcher    69338c3d0b556af446bba3fb87ca61fcdbcb8ff327a648cd85f1832238438d5e    /Users/jam/.notary/private
```
Export the root key.  Remove the root key and keep it somewhere safe along with the pass phrases you defined earlier. DO NOT LOSE IT! There is no way to recover it if it is lost.
See [Notary Documentation](https://github.com/docker/notary/blob/master/docs/advanced_usage.md) for tips on key management. The root key should not be
stored on developers workstations unless it is needed to create repositories.
```
notary key export --key b8dc5cded1a8522a563a58c3ac7ad2eba51d6945999aa5864678fb5064bb6f9e -o notary-root.key
```
Also export the snapshot and target key.  These keys will be needed along with their respective pass phrases to publish content to Notary.
```
notary key export --key  5e1221edd379be729f12f3cb69786758ee23a71067b6e25c62d10ccfe0c82f31 -o snapshot.pem                                                                                                                                             
notary key export --key  69338c3d0b556af446bba3fb87ca61fcdbcb8ff327a648cd85f1832238438d5e  -o targets.pem
```
In order to publish releases to Notary using the mirror command the following environment variables must be defined with pass phrases for respective Notary keys.
```
NOTARY_SNAPSHOT_PASSPHRASE=<snapshot secret>
NOTARY_TARGETS_PASSPHRASE=<targets secret>
```
The publisher will also need to import the latest targets and snapshot keys into their local Notary
configuration if they are not already present. `notary key list` will indicate if the needed
keys are present. Look for the targets and snapshot key for the GUN you'll be publishing to. If not present
the following command will import the keys.
```
notary key import snapshot.pem targets.pem
```
#### Usage

The `mirror` command may be used to do all or some subset of the following actions:

- Produce archives for both Launcher and Osqueryd
- Upload the archives to the highly-available GCP storage bucket
- Register the releases with https://notary.kolide.com so that they can be validated as part of the Launcher's autoupdate process

By default, the `-all` flag is set, which will perform all the steps needed to release new binaries. If you set `-all=false`, individual subcommands can be used to perform a subset of these operations. For a full list of these subcommands, run `package-builder mirror --help`:

```
$ ./build/package-builder mirror --help
USAGE
  package-builder mirror [flags]

FLAGS
  -all true                Complete build and publish of Osquery and Launcher. If false, operations are enabled individually.
  -channel stable          Create a tarball for a specific autoupdate channel. Valid values: beta, stable and nightly.
  -debug false             Enable debug logging.
  -download false          Download a fresh copy of Osquery from s3.
  -extract false           Extract Osquery binary from package.
  -launcher-publish false  Publish Launcher tarball to Notary.
  -launcher-tarball false  Create a tarball from Launcher build.
  -launcher-upload false   Upload Launcher tarball to mirror.
  -osquery-publish false   Publish Osquery target to Notary.
  -osquery-tarball false   Create a tarball from Osquery binary.
  -osquery-upload false    Upload Osquery tarball to mirror.
  -platform darwin         Platform to build. Valid values are darwin, linux and windows.
```

For example, you could choose to publish changes for Osquery or only create archives for Launcher. The following commands would:

- Download and publish the latest version of Osquery
- Publish version 1.2.0 of Launcher to the `stable` channel for the `linux` platform.

```
git tag 1.2.0
gcloud config set project kolide-website
make package-builder
build/package-builder mirror -platform=linux
```

#### Notary

You can use the Notary Client to list the signatures of osqueryd and launcher versions that have been published:

```
$ notary list kolide/osqueryd
NAME                             DIGEST                                                              SIZE (BYTES)    ROLE
----                             ------                                                              ------------    ----
darwin/osqueryd-2.7.0.tar.gz     ea4efa14edbf8e7d63de2af1cbb8dc6be200b8cb8e76731c04827b40760e6cfc    4252776         targets
darwin/osqueryd-stable.tar.gz    ea4efa14edbf8e7d63de2af1cbb8dc6be200b8cb8e76731c04827b40760e6cfc    4252776         targets
```

#### Storage

Osquery and Launcher binaries must be available in a .tar.gz archive, on a remote mirror so the Launcher can download them. The `package-builder mirror` command packages binaries into archives and publishes them in the format `<binary-name>-<update-channel or version>.tar.gz`

For example:

```
osqueryd-stable.tar.gz
osqueryd-2.6.0.tar.gz
launcher-stable.tar.gz
launcher-874e302.tar.gz
```

The archives are stored in GCP by Kolide (`gs://binaries-for-launcher`) and they are exposed at a URI formatted like:

```
https://dl.kolide.com/kolide/<binary>/<platform>/<archive>
```

For example: https://dl.kolide.com/kolide/osqueryd/darwin/osqueryd-stable.tar.gz
