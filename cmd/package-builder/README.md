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

By default, binaries will be installed to `/usr/local/launcher/bin`, configuration will be installed to `/etc/launcher`, logs will be outputted to `/var/log/launcher`, etc. If you'd like the `launcher` string to be something else (for example, your company name), you can use the `--identifier` flag to specify this value.

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
  --mac_package_signing_key="Developer ID Installer: Acme Inc (ABCDEF123456)" \
	--pr_start=600 \
	--pr_end=750
```

### Production Packages

To use the tool to generate Kolide production packages, run:

```
./build/package-builder prod --debug \
  --enrollment_secret_signing_key=/path/to/key.pem \
  --mac_package_signing_key="Developer ID Installer: Acme Inc (ABCDEF123456)"
```

### Publishing Updates

The `mirror` command may be used to do all or some subset of the following actions:

* Produce archives for both Launcher and Osqueryd
* Upload the archives to the highly-available GCP storage bucket
* Register the releases with https://notary.kolide.com so that they can be validated as part of the Launcher's autoupdate process

```
build/package-builder mirror -help

USAGE
  package-builder mirror [flags]

FLAGS
  -all false               Complete build and publish of Osquery and Launcher. If false, operations are enabled individually.
  -channel stable          Create a tarball for a specific autoupdate channel. Valid values: beta, stable and nightly.
  -debug false             Enable debug logging.
  -download false          Download a fresh copy of Osquery from s3.
  -extract false           Extract Osquery binary from package.
  -launcher-all false      Complete build and publish of Launcher
  -launcher-publish false  Publish Launcher tarball to Notary.
  -launcher-tarball false  Create a tarball from Launcher build.
  -launcher-upload false   Upload Launcher tarball to mirror.
  -osquery-all false       Complete build and publish of Osquery
  -osquery-publish false   Publish Osquery target to Notary.
  -osquery-tarball false   Create a tarball from Osquery binary.
  -osquery-upload false    Upload Osquery tarball to mirror.
  -platform darwin         Platform to build. Valid values are darwin, linux and windows.
```

#### Examples

Publish archives containing latest version of Launcher and Osqueryd. Launcher instances that have autoupdate enabled will pick up and install these versions of Launcher and Osqueryd.

```
build/package-builder mirror -all -debug
```

Publish archive containing the latest version of Launcher for autoupdate.

```
build/package-builder mirror -launcher-all
```

Publish archive containing latest version of Osquery for autoupdate.

```
build/package-builder mirror -osquery-all
```

Suppose that we want to download the latest version of Osquery and create a distribution tarball for testing. We don't want to upload or publish the tarball. The mirror command makes this possible; however if we don't use the **all** commands we must take care to invoke all the prerequisites ourselves.  For example supplying the `-osquery-tarball` command alone will cause the following error because we must download the Osquery package and extract it before we create the tarball.

```
build/package-builder mirror -osquery-tarball
{"caller":"mirror.go:228","err":"missing predecessor","level":"error","method":"Publish","ts":"2017-09-11T20:13:42.815424865Z"}
missing predecessor
```

In order to create the tarball, we first need to download the Osquery packages and then extract the `osqueryd` binary that we are interested in. The following command works.

```
build/package-builder mirror -osquery-tarball -download -extract
{"caller":"mirror.go:336","level":"info","msg":"completed downloading osquery","ts":"2017-09-11T20:17:46.539922529Z"}
{"caller":"mirror.go:371","level":"info","msg":"completed extract osquery","ts":"2017-09-11T20:17:46.785266738Z"}
{"caller":"mirror.go:310","level":"info","msg":"get osquery version","ts":"2017-09-11T20:17:46.840920175Z","version":"2.7.0"}
{"caller":"mirror.go:445","level":"info","msg":"generate osquery tarball","output":"/tmp/osquery_mirror/binaries-for-launcher/kolide/osqueryd/darwin/osqueryd-2.7.0.tar.gz","ts":"2017-09-11T20:17:47.331453869Z"}
{"caller":"mirror.go:450","level":"info","msg":"generate osquery tarball","output":"/tmp/osquery_mirror/binaries-for-launcher/kolide/osqueryd/darwin/osqueryd-2.7.0.tar.gz","ts":"2017-09-11T20:17:47.331513564Z"}
{"caller":"mirror.go:486","level":"info","msg":"generated osquery tagged tarball","output":"/tmp/osquery_mirror/binaries-for-launcher/kolide/osqueryd/darwin/osqueryd-stable.tar.gz","ts":"2017-09-11T20:17:47.342412008Z"}
```

In this Launcher example we create a distribution tarball and upload it to the mirror.

```
build/package-builder mirror -launcher-tarball -launcher-upload
```

#### Prerequisites

To use this command, you must be authorized using `gcloud` and be configured to use the `kolide-website` GCP project.

```
gcloud auth application-default login
gcloud config set project kolide-website
```

In addition to GCP, the [Notary command-line client](https://github.com/docker/notary) must be configured to communicate with the Kolide notary server. Delegate keys must be installed and passphrases must be available. See the next section that describes setting up Notary if the TUF respositories haven't been created yet.

Setup the Notary client configuration.

```
mkdir ~/.notary && echo '{ "remote_server": { "url": "https://notary.kolide.com" } }' > ~/.notary/config.json
```

Set the delegation environment variable:

```
export NOTARY_DELEGATION_PASSPHRASE=<secret>
```

Import the delegate keys. This will authorize you to use your local Notary client to publish updates.

```
notary key import launcher-key.pem --role targets/releases --gun kolide/launcher
notary key import osqueryd-key.pem --role targets/releases --gun kolide/osqueryd
```

#### Creating a new TUF Repository

This section is documentation on how the `kolide/osqueryd` and `kolide/launcher` TUF repositories were setup. This information will be useful when re-configuring a Notary server, creating new TUF repositories, etc. The [initial set up for Notary](https://github.com/kolide/updater) should be completed prior to setting up repositories and is beyond the scope of this document. If Notary is already set up you're ready to set up the new repositories.

The first step is to select strong passphrases and assign them to the following environment variables:

```
export NOTARY_DELEGATION_PASSPHRASE=<secret>
export NOTARY_ROOT_PASSPHRASE=<secret>
export NOTARY_SNAPSHOT_PASSPHRASE=<secret>
export NOTARY_TARGETS_PASSPHRASE=<secret>
```

Create GUNs (Global Unique Identifiers) for the repositories.

```
notary init kolide/launcher -p
notary init kolide/osqueryd -p
```

Rotate snapshot keys so that they are managed by Notary server.

```
notary key rotate kolide/launcher snapshot -r
notary key rotate kolide/osqueryd snapshot -r
```

Create keys for delegates. This process will create two x509 certs, `launcher.pem` and `osqueryd.pem`.  It will also create private keys `launcher-key.pem` and `osqueryd-key.pem`.

```
notary key generate ecdsa --role targets/releases -o launcher
notary key generate ecdsa --role targets/releases -o osqueryd
```

Create the delegates, importing the x509 certificates created in the previous step.

```
notary delegation add kolide/launcher targets/releases launcher.pem --all-paths -p
notary delegation add kolide/osqueryd targets/releases osqueryd.pem --all-paths -p
```

Modify the path header of each private key adding the key ID of the associated delegate key. Do this for both `kolide/launcher` and `kolide/osqueryd`. Find the delegate key using `notary delegate list` as in the following example.

```
notary delegation list kolide/launcher

ROLE                PATHS             KEY IDS                                                             THRESHOLD
----                -----             -------                                                             ---------
targets/releases    "" <all paths>    06061078b3fefc16d5170cdfc3af6e8881d2d4a283e7a7b894c89402e3a5057d    1
```

Open the private key you created for example `launcher-key.pem` in a text editor and add the Key ID to the path header of the key.

```
-----BEGIN EC PRIVATE KEY-----
Proc-Type: 4,ENCRYPTED
DEK-Info: AES-256-CBC,f6aa527f4df1bf0586e5c78a5cf391bc
role: targets/releases
path: 06061078b3fefc16d5170cdfc3af6e8881d2d4a283e7a7b894c89402e3a5057d

y7yWNcOBsMiY7owqkXVKEzmlIJ4czs2t+oB7MceX7WZrxI3O51Fr2YYX7Q5+jiZF
iI1fszTUNu8f07bY/u0c36K6LiTQOIxiT5N2YMD5+sb4XRE9KUpSSOEVEWlMGopw
Xm//qxWRIzC4C5Tc11liQ9gfz3PJ3TX2gOoQJMtfq6k=
-----END EC PRIVATE KEY-----
```

The delegate keys and passphrases should all be stored safely offline so they are available set up Notary Client to publish updates.
